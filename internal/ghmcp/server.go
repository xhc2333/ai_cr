package ghmcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/github/github-mcp-server/pkg/errors"
	"github.com/github/github-mcp-server/pkg/github"
	mcplog "github.com/github/github-mcp-server/pkg/log"
	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/github/github-mcp-server/pkg/translations"
	gogithub "github.com/google/go-github/v74/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/shurcooL/githubv4"
)

type MCPServerConfig struct {
	// Version of the server
	Version string

	// GitHub Host to target for API requests (e.g. github.com or github.enterprise.com)
	Host string

	// GitHub Token to authenticate with the GitHub API
	Token string

	// EnabledToolsets is a list of toolsets to enable
	// See: https://github.com/github/github-mcp-server?tab=readme-ov-file#tool-configuration
	EnabledToolsets []string

	// Whether to enable dynamic toolsets
	// See: https://github.com/github/github-mcp-server?tab=readme-ov-file#dynamic-tool-discovery
	DynamicToolsets bool

	// ReadOnly indicates if we should only offer read-only tools
	ReadOnly bool

	// Translator provides translated text for the server tooling
	Translator translations.TranslationHelperFunc

	// Content window size
	ContentWindowSize int
}

const stdioServerLogPrefix = "stdioserver"

func NewMCPServer(cfg MCPServerConfig) (*server.MCPServer, error) {
	apiHost, err := parseAPIHost(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API host: %w", err)
	}

	// Construct our REST client
	restClient := gogithub.NewClient(nil).WithAuthToken(cfg.Token)
	restClient.UserAgent = fmt.Sprintf("github-mcp-server/%s", cfg.Version)
	restClient.BaseURL = apiHost.baseRESTURL
	restClient.UploadURL = apiHost.uploadURL

	// Construct our GraphQL client
	// We're using NewEnterpriseClient here unconditionally as opposed to NewClient because we already
	// did the necessary API host parsing so that github.com will return the correct URL anyway.
	gqlHTTPClient := &http.Client{
		Transport: &bearerAuthTransport{
			transport: http.DefaultTransport,
			token:     cfg.Token,
		},
	} // We're going to wrap the Transport later in beforeInit
	gqlClient := githubv4.NewEnterpriseClient(apiHost.graphqlURL.String(), gqlHTTPClient)

	// When a client send an initialize request, update the user agent to include the client info.
	beforeInit := func(_ context.Context, _ any, message *mcp.InitializeRequest) {
		userAgent := fmt.Sprintf(
			"github-mcp-server/%s (%s/%s)",
			cfg.Version,
			message.Params.ClientInfo.Name,
			message.Params.ClientInfo.Version,
		)

		restClient.UserAgent = userAgent

		gqlHTTPClient.Transport = &userAgentTransport{
			transport: gqlHTTPClient.Transport,
			agent:     userAgent,
		}
	}

	hooks := &server.Hooks{
		OnBeforeInitialize: []server.OnBeforeInitializeFunc{beforeInit},
		OnBeforeAny: []server.BeforeAnyHookFunc{
			func(ctx context.Context, _ any, _ mcp.MCPMethod, _ any) {
				// Ensure the context is cleared of any previous errors
				// as context isn't propagated through middleware
				errors.ContextWithGitHubErrors(ctx)
			},
		},
	}

	enabledToolsets := cfg.EnabledToolsets
	if cfg.DynamicToolsets {
		// filter "all" from the enabled toolsets
		enabledToolsets = make([]string, 0, len(cfg.EnabledToolsets))
		for _, toolset := range cfg.EnabledToolsets {
			if toolset != "all" {
				enabledToolsets = append(enabledToolsets, toolset)
			}
		}
	}

	// Generate instructions based on enabled toolsets
	instructions := github.GenerateInstructions(enabledToolsets)
	
	ghServer := github.NewServer(cfg.Version, 
		server.WithInstructions(instructions),
		server.WithHooks(hooks),
	)

	getClient := func(_ context.Context) (*gogithub.Client, error) {
		return restClient, nil // closing over client
	}

	getGQLClient := func(_ context.Context) (*githubv4.Client, error) {
		return gqlClient, nil // closing over client
	}

	getRawClient := func(ctx context.Context) (*raw.Client, error) {
		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}
		return raw.NewClient(client, apiHost.rawURL), nil // closing over client
	}

	// Create default toolsets
	tsg := github.DefaultToolsetGroup(cfg.ReadOnly, getClient, getGQLClient, getRawClient, cfg.Translator, cfg.ContentWindowSize)
	err = tsg.EnableToolsets(enabledToolsets)

	if err != nil {
		return nil, fmt.Errorf("failed to enable toolsets: %w", err)
	}

	// Register all mcp functionality with the server
	tsg.RegisterAll(ghServer)

	if cfg.DynamicToolsets {
		dynamic := github.InitDynamicToolset(ghServer, tsg, cfg.Translator)
		dynamic.RegisterTools(ghServer)
	}

	return ghServer, nil
}

type StdioServerConfig struct {
	// Version of the server
	Version string

	// GitHub Host to target for API requests (e.g. github.com or github.enterprise.com)
	Host string

	// GitHub Token to authenticate with the GitHub API
	Token string

	// EnabledToolsets is a list of toolsets to enable
	// See: https://github.com/github/github-mcp-server?tab=readme-ov-file#tool-configuration
	EnabledToolsets []string

	// Whether to enable dynamic toolsets
	// See: https://github.com/github/github-mcp-server?tab=readme-ov-file#dynamic-tool-discovery
	DynamicToolsets bool

	// ReadOnly indicates if we should only register read-only tools
	ReadOnly bool

	// ExportTranslations indicates if we should export translations
	// See: https://github.com/github/github-mcp-server?tab=readme-ov-file#i18n--overriding-descriptions
	ExportTranslations bool

	// EnableCommandLogging indicates if we should log commands
	EnableCommandLogging bool

	// Path to the log file if not stderr
	LogFilePath string

	// Content window size
	ContentWindowSize int
}

// RunStdioServer is not concurrent safe.
func RunStdioServer(cfg StdioServerConfig) error {
	// Create app context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	t, dumpTranslations := translations.TranslationHelper()

	ghServer, err := NewMCPServer(MCPServerConfig{
		Version:           cfg.Version,
		Host:              cfg.Host,
		Token:             cfg.Token,
		EnabledToolsets:   cfg.EnabledToolsets,
		DynamicToolsets:   cfg.DynamicToolsets,
		ReadOnly:          cfg.ReadOnly,
		Translator:        t,
		ContentWindowSize: cfg.ContentWindowSize,
	})
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	stdioServer := server.NewStdioServer(ghServer)

	var slogHandler slog.Handler
	var logOutput io.Writer
	if cfg.LogFilePath != "" {
		file, err := os.OpenFile(cfg.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		logOutput = file
		slogHandler = slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		logOutput = os.Stderr
		slogHandler = slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	logger := slog.New(slogHandler)
	logger.Info("starting server", "version", cfg.Version, "host", cfg.Host, "dynamicToolsets", cfg.DynamicToolsets, "readOnly", cfg.ReadOnly)
	stdLogger := log.New(logOutput, stdioServerLogPrefix, 0)
	stdioServer.SetErrorLogger(stdLogger)

	if cfg.ExportTranslations {
		// Once server is initialized, all translations are loaded
		dumpTranslations()
	}

	// Start listening for messages
	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)

		if cfg.EnableCommandLogging {
			loggedIO := mcplog.NewIOLogger(in, out, logger)
			in, out = loggedIO, loggedIO
		}
		// enable GitHub errors in the context
		ctx := errors.ContextWithGitHubErrors(ctx)
		errC <- stdioServer.Listen(ctx, in, out)
	}()

	// Output github-mcp-server string
	_, _ = fmt.Fprintf(os.Stderr, "GitHub MCP Server running on stdio\n")

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		logger.Info("shutting down server", "signal", "context done")
	case err := <-errC:
		if err != nil {
			logger.Error("error running server", "error", err)
			return fmt.Errorf("error running server: %w", err)
		}
	}

	return nil
}

type apiHost struct {
	baseRESTURL *url.URL
	graphqlURL  *url.URL
	uploadURL   *url.URL
	rawURL      *url.URL
}

func newDotcomHost() (apiHost, error) {
	baseRestURL, err := url.Parse("https://api.github.com/")
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse dotcom REST URL: %w", err)
	}

	gqlURL, err := url.Parse("https://api.github.com/graphql")
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse dotcom GraphQL URL: %w", err)
	}

	uploadURL, err := url.Parse("https://uploads.github.com")
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse dotcom Upload URL: %w", err)
	}

	rawURL, err := url.Parse("https://raw.githubusercontent.com/")
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse dotcom Raw URL: %w", err)
	}

	return apiHost{
		baseRESTURL: baseRestURL,
		graphqlURL:  gqlURL,
		uploadURL:   uploadURL,
		rawURL:      rawURL,
	}, nil
}

func newGHECHost(hostname string) (apiHost, error) {
	u, err := url.Parse(hostname)
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHEC URL: %w", err)
	}

	// Unsecured GHEC would be an error
	if u.Scheme == "http" {
		return apiHost{}, fmt.Errorf("GHEC URL must be HTTPS")
	}

	restURL, err := url.Parse(fmt.Sprintf("https://api.%s/", u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHEC REST URL: %w", err)
	}

	gqlURL, err := url.Parse(fmt.Sprintf("https://api.%s/graphql", u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHEC GraphQL URL: %w", err)
	}

	uploadURL, err := url.Parse(fmt.Sprintf("https://uploads.%s", u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHEC Upload URL: %w", err)
	}

	rawURL, err := url.Parse(fmt.Sprintf("https://raw.%s/", u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHEC Raw URL: %w", err)
	}

	return apiHost{
		baseRESTURL: restURL,
		graphqlURL:  gqlURL,
		uploadURL:   uploadURL,
		rawURL:      rawURL,
	}, nil
}

func newGHESHost(hostname string) (apiHost, error) {
	u, err := url.Parse(hostname)
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHES URL: %w", err)
	}

	restURL, err := url.Parse(fmt.Sprintf("%s://%s/api/v3/", u.Scheme, u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHES REST URL: %w", err)
	}

	gqlURL, err := url.Parse(fmt.Sprintf("%s://%s/api/graphql", u.Scheme, u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHES GraphQL URL: %w", err)
	}

	uploadURL, err := url.Parse(fmt.Sprintf("%s://%s/api/uploads/", u.Scheme, u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHES Upload URL: %w", err)
	}
	rawURL, err := url.Parse(fmt.Sprintf("%s://%s/raw/", u.Scheme, u.Hostname()))
	if err != nil {
		return apiHost{}, fmt.Errorf("failed to parse GHES Raw URL: %w", err)
	}

	return apiHost{
		baseRESTURL: restURL,
		graphqlURL:  gqlURL,
		uploadURL:   uploadURL,
		rawURL:      rawURL,
	}, nil
}

// Note that this does not handle ports yet, so development environments are out.
func parseAPIHost(s string) (apiHost, error) {
	if s == "" {
		return newDotcomHost()
	}

	u, err := url.Parse(s)
	if err != nil {
		return apiHost{}, fmt.Errorf("could not parse host as URL: %s", s)
	}

	if u.Scheme == "" {
		return apiHost{}, fmt.Errorf("host must have a scheme (http or https): %s", s)
	}

	if strings.HasSuffix(u.Hostname(), "github.com") {
		return newDotcomHost()
	}

	if strings.HasSuffix(u.Hostname(), "ghe.com") {
		return newGHECHost(s)
	}

	return newGHESHost(s)
}

type userAgentTransport struct {
	transport http.RoundTripper
	agent     string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", t.agent)
	return t.transport.RoundTrip(req)
}

type bearerAuthTransport struct {
	transport http.RoundTripper
	token     string
}

func (t *bearerAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.transport.RoundTrip(req)
}
