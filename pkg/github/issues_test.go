package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/github/github-mcp-server/internal/githubv4mock"
	"github.com/github/github-mcp-server/internal/toolsnaps"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v74/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetIssue(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := GetIssue(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "get_issue", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number"})

	// Setup mock issue for success case
	mockIssue := &github.Issue{
		Number:  github.Ptr(42),
		Title:   github.Ptr("Test Issue"),
		Body:    github.Ptr("This is a test issue"),
		State:   github.Ptr("open"),
		HTMLURL: github.Ptr("https://github.com/owner/repo/issues/42"),
		User: &github.User{
			Login: github.Ptr("testuser"),
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedIssue  *github.Issue
		expectedErrMsg string
	}{
		{
			name: "successful issue retrieval",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposIssuesByOwnerByRepoByIssueNumber,
					mockIssue,
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name: "issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Issue not found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
			},
			expectError:    true,
			expectedErrMsg: "failed to get issue",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := GetIssue(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedIssue github.Issue
			err = json.Unmarshal([]byte(textContent.Text), &returnedIssue)
			require.NoError(t, err)
			assert.Equal(t, *tc.expectedIssue.Number, *returnedIssue.Number)
			assert.Equal(t, *tc.expectedIssue.Title, *returnedIssue.Title)
			assert.Equal(t, *tc.expectedIssue.Body, *returnedIssue.Body)
			assert.Equal(t, *tc.expectedIssue.State, *returnedIssue.State)
			assert.Equal(t, *tc.expectedIssue.HTMLURL, *returnedIssue.HTMLURL)
			assert.Equal(t, *tc.expectedIssue.User.Login, *returnedIssue.User.Login)
		})
	}
}

func Test_AddIssueComment(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := AddIssueComment(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "add_issue_comment", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.Contains(t, tool.InputSchema.Properties, "body")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number", "body"})

	// Setup mock comment for success case
	mockComment := &github.IssueComment{
		ID:   github.Ptr(int64(123)),
		Body: github.Ptr("This is a test comment"),
		User: &github.User{
			Login: github.Ptr("testuser"),
		},
		HTMLURL: github.Ptr("https://github.com/owner/repo/issues/42#issuecomment-123"),
	}

	tests := []struct {
		name            string
		mockedClient    *http.Client
		requestArgs     map[string]interface{}
		expectError     bool
		expectedComment *github.IssueComment
		expectedErrMsg  string
	}{
		{
			name: "successful comment creation",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesCommentsByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusCreated, mockComment),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"body":         "This is a test comment",
			},
			expectError:     false,
			expectedComment: mockComment,
		},
		{
			name: "comment creation fails",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesCommentsByOwnerByRepoByIssueNumber,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusUnprocessableEntity)
						_, _ = w.Write([]byte(`{"message": "Invalid request"}`))
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"body":         "",
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: body",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := AddIssueComment(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			if tc.expectedErrMsg != "" {
				require.NotNil(t, result)
				textContent := getTextResult(t, result)
				assert.Contains(t, textContent.Text, tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)

			// Parse the result and get the text content if no error
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedComment github.IssueComment
			err = json.Unmarshal([]byte(textContent.Text), &returnedComment)
			require.NoError(t, err)
			assert.Equal(t, *tc.expectedComment.ID, *returnedComment.ID)
			assert.Equal(t, *tc.expectedComment.Body, *returnedComment.Body)
			assert.Equal(t, *tc.expectedComment.User.Login, *returnedComment.User.Login)

		})
	}
}

func Test_SearchIssues(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := SearchIssues(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "search_issues", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "query")
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "sort")
	assert.Contains(t, tool.InputSchema.Properties, "order")
	assert.Contains(t, tool.InputSchema.Properties, "perPage")
	assert.Contains(t, tool.InputSchema.Properties, "page")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"query"})

	// Setup mock search results
	mockSearchResult := &github.IssuesSearchResult{
		Total:             github.Ptr(2),
		IncompleteResults: github.Ptr(false),
		Issues: []*github.Issue{
			{
				Number:   github.Ptr(42),
				Title:    github.Ptr("Bug: Something is broken"),
				Body:     github.Ptr("This is a bug report"),
				State:    github.Ptr("open"),
				HTMLURL:  github.Ptr("https://github.com/owner/repo/issues/42"),
				Comments: github.Ptr(5),
				User: &github.User{
					Login: github.Ptr("user1"),
				},
			},
			{
				Number:   github.Ptr(43),
				Title:    github.Ptr("Feature: Add new functionality"),
				Body:     github.Ptr("This is a feature request"),
				State:    github.Ptr("open"),
				HTMLURL:  github.Ptr("https://github.com/owner/repo/issues/43"),
				Comments: github.Ptr(3),
				User: &github.User{
					Login: github.Ptr("user2"),
				},
			},
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedResult *github.IssuesSearchResult
		expectedErrMsg string
	}{
		{
			name: "successful issues search with all parameters",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "is:issue repo:owner/repo is:open",
							"sort":     "created",
							"order":    "desc",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query":   "repo:owner/repo is:open",
				"sort":    "created",
				"order":   "desc",
				"page":    float64(1),
				"perPage": float64(30),
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "issues search with owner and repo parameters",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "repo:test-owner/test-repo is:issue is:open",
							"sort":     "created",
							"order":    "asc",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "is:open",
				"owner": "test-owner",
				"repo":  "test-repo",
				"sort":  "created",
				"order": "asc",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "issues search with only owner parameter (should ignore it)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "is:issue bug",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "bug",
				"owner": "test-owner",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "issues search with only repo parameter (should ignore it)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "is:issue feature",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "feature",
				"repo":  "test-repo",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "issues search with minimal parameters",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetSearchIssues,
					mockSearchResult,
				),
			),
			requestArgs: map[string]interface{}{
				"query": "is:issue repo:owner/repo is:open",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "query with existing is:issue filter - no duplication",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "repo:github/github-mcp-server is:issue is:open (label:critical OR label:urgent)",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "repo:github/github-mcp-server is:issue is:open (label:critical OR label:urgent)",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "query with existing repo: filter and conflicting owner/repo params - uses query filter",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "is:issue repo:github/github-mcp-server critical",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "repo:github/github-mcp-server critical",
				"owner": "different-owner",
				"repo":  "different-repo",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "query with both is: and repo: filters already present",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "is:issue repo:octocat/Hello-World bug",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "is:issue repo:octocat/Hello-World bug",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "complex query with multiple OR operators and existing filters",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					expectQueryParams(
						t,
						map[string]string{
							"q":        "repo:github/github-mcp-server is:issue (label:critical OR label:urgent OR label:high-priority OR label:blocker)",
							"page":     "1",
							"per_page": "30",
						},
					).andThen(
						mockResponse(t, http.StatusOK, mockSearchResult),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "repo:github/github-mcp-server is:issue (label:critical OR label:urgent OR label:high-priority OR label:blocker)",
			},
			expectError:    false,
			expectedResult: mockSearchResult,
		},
		{
			name: "search issues fails",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetSearchIssues,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"message": "Validation Failed"}`))
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"query": "invalid:query",
			},
			expectError:    true,
			expectedErrMsg: "failed to search issues",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := SearchIssues(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)

			// Parse the result and get the text content if no error
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedResult github.IssuesSearchResult
			err = json.Unmarshal([]byte(textContent.Text), &returnedResult)
			require.NoError(t, err)
			assert.Equal(t, *tc.expectedResult.Total, *returnedResult.Total)
			assert.Equal(t, *tc.expectedResult.IncompleteResults, *returnedResult.IncompleteResults)
			assert.Len(t, returnedResult.Issues, len(tc.expectedResult.Issues))
			for i, issue := range returnedResult.Issues {
				assert.Equal(t, *tc.expectedResult.Issues[i].Number, *issue.Number)
				assert.Equal(t, *tc.expectedResult.Issues[i].Title, *issue.Title)
				assert.Equal(t, *tc.expectedResult.Issues[i].State, *issue.State)
				assert.Equal(t, *tc.expectedResult.Issues[i].HTMLURL, *issue.HTMLURL)
				assert.Equal(t, *tc.expectedResult.Issues[i].User.Login, *issue.User.Login)
			}
		})
	}
}

func Test_CreateIssue(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := CreateIssue(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "create_issue", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "title")
	assert.Contains(t, tool.InputSchema.Properties, "body")
	assert.Contains(t, tool.InputSchema.Properties, "assignees")
	assert.Contains(t, tool.InputSchema.Properties, "labels")
	assert.Contains(t, tool.InputSchema.Properties, "milestone")
	assert.Contains(t, tool.InputSchema.Properties, "type")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "title"})

	// Setup mock issue for success case
	mockIssue := &github.Issue{
		Number:    github.Ptr(123),
		Title:     github.Ptr("Test Issue"),
		Body:      github.Ptr("This is a test issue"),
		State:     github.Ptr("open"),
		HTMLURL:   github.Ptr("https://github.com/owner/repo/issues/123"),
		Assignees: []*github.User{{Login: github.Ptr("user1")}, {Login: github.Ptr("user2")}},
		Labels:    []*github.Label{{Name: github.Ptr("bug")}, {Name: github.Ptr("help wanted")}},
		Milestone: &github.Milestone{Number: github.Ptr(5)},
		Type:      &github.IssueType{Name: github.Ptr("Bug")},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedIssue  *github.Issue
		expectedErrMsg string
	}{
		{
			name: "successful issue creation with all fields",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					expectRequestBody(t, map[string]any{
						"title":     "Test Issue",
						"body":      "This is a test issue",
						"labels":    []any{"bug", "help wanted"},
						"assignees": []any{"user1", "user2"},
						"milestone": float64(5),
						"type":      "Bug",
					}).andThen(
						mockResponse(t, http.StatusCreated, mockIssue),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":     "owner",
				"repo":      "repo",
				"title":     "Test Issue",
				"body":      "This is a test issue",
				"assignees": []any{"user1", "user2"},
				"labels":    []any{"bug", "help wanted"},
				"milestone": float64(5),
				"type":      "Bug",
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name: "successful issue creation with minimal fields",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					mockResponse(t, http.StatusCreated, &github.Issue{
						Number:  github.Ptr(124),
						Title:   github.Ptr("Minimal Issue"),
						HTMLURL: github.Ptr("https://github.com/owner/repo/issues/124"),
						State:   github.Ptr("open"),
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":     "owner",
				"repo":      "repo",
				"title":     "Minimal Issue",
				"assignees": nil, // Expect no failure with nil optional value.
			},
			expectError: false,
			expectedIssue: &github.Issue{
				Number:  github.Ptr(124),
				Title:   github.Ptr("Minimal Issue"),
				HTMLURL: github.Ptr("https://github.com/owner/repo/issues/124"),
				State:   github.Ptr("open"),
			},
		},
		{
			name: "issue creation fails",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusUnprocessableEntity)
						_, _ = w.Write([]byte(`{"message": "Validation failed"}`))
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"title": "",
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: title",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := CreateIssue(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			if tc.expectedErrMsg != "" {
				require.NotNil(t, result)
				textContent := getTextResult(t, result)
				assert.Contains(t, textContent.Text, tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			textContent := getTextResult(t, result)

			// Unmarshal and verify the minimal result
			var returnedIssue MinimalResponse
			err = json.Unmarshal([]byte(textContent.Text), &returnedIssue)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedIssue.GetHTMLURL(), returnedIssue.URL)
		})
	}
}

func Test_ListIssues(t *testing.T) {
	// Verify tool definition
	mockClient := githubv4.NewClient(nil)
	tool, _ := ListIssues(stubGetGQLClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "list_issues", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "state")
	assert.Contains(t, tool.InputSchema.Properties, "labels")
	assert.Contains(t, tool.InputSchema.Properties, "orderBy")
	assert.Contains(t, tool.InputSchema.Properties, "direction")
	assert.Contains(t, tool.InputSchema.Properties, "since")
	assert.Contains(t, tool.InputSchema.Properties, "after")
	assert.Contains(t, tool.InputSchema.Properties, "perPage")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo"})

	// Mock issues data
	mockIssuesAll := []map[string]any{
		{
			"number":     123,
			"title":      "First Issue",
			"body":       "This is the first test issue",
			"state":      "OPEN",
			"databaseId": 1001,
			"createdAt":  "2023-01-01T00:00:00Z",
			"updatedAt":  "2023-01-01T00:00:00Z",
			"author":     map[string]any{"login": "user1"},
			"labels": map[string]any{
				"nodes": []map[string]any{
					{"name": "bug", "id": "label1", "description": "Bug label"},
				},
			},
			"comments": map[string]any{
				"totalCount": 5,
			},
		},
		{
			"number":     456,
			"title":      "Second Issue",
			"body":       "This is the second test issue",
			"state":      "OPEN",
			"databaseId": 1002,
			"createdAt":  "2023-02-01T00:00:00Z",
			"updatedAt":  "2023-02-01T00:00:00Z",
			"author":     map[string]any{"login": "user2"},
			"labels": map[string]any{
				"nodes": []map[string]any{
					{"name": "enhancement", "id": "label2", "description": "Enhancement label"},
				},
			},
			"comments": map[string]any{
				"totalCount": 3,
			},
		},
	}

	mockIssuesOpen := []map[string]any{mockIssuesAll[0], mockIssuesAll[1]}
	mockIssuesClosed := []map[string]any{
		{
			"number":     789,
			"title":      "Closed Issue",
			"body":       "This is a closed issue",
			"state":      "CLOSED",
			"databaseId": 1003,
			"createdAt":  "2023-03-01T00:00:00Z",
			"updatedAt":  "2023-03-01T00:00:00Z",
			"author":     map[string]any{"login": "user3"},
			"labels": map[string]any{
				"nodes": []map[string]any{},
			},
			"comments": map[string]any{
				"totalCount": 1,
			},
		},
	}

	// Mock responses
	mockResponseListAll := githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"issues": map[string]any{
				"nodes": mockIssuesAll,
				"pageInfo": map[string]any{
					"hasNextPage":     false,
					"hasPreviousPage": false,
					"startCursor":     "",
					"endCursor":       "",
				},
				"totalCount": 2,
			},
		},
	})

	mockResponseOpenOnly := githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"issues": map[string]any{
				"nodes": mockIssuesOpen,
				"pageInfo": map[string]any{
					"hasNextPage":     false,
					"hasPreviousPage": false,
					"startCursor":     "",
					"endCursor":       "",
				},
				"totalCount": 2,
			},
		},
	})

	mockResponseClosedOnly := githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"issues": map[string]any{
				"nodes": mockIssuesClosed,
				"pageInfo": map[string]any{
					"hasNextPage":     false,
					"hasPreviousPage": false,
					"startCursor":     "",
					"endCursor":       "",
				},
				"totalCount": 1,
			},
		},
	})

	mockErrorRepoNotFound := githubv4mock.ErrorResponse("repository not found")

	// Variables matching what GraphQL receives after JSON marshaling/unmarshaling
	varsListAll := map[string]interface{}{
		"owner":     "owner",
		"repo":      "repo",
		"states":    []interface{}{"OPEN", "CLOSED"},
		"orderBy":   "CREATED_AT",
		"direction": "DESC",
		"first":     float64(30),
		"after":     (*string)(nil),
	}

	varsOpenOnly := map[string]interface{}{
		"owner":     "owner",
		"repo":      "repo",
		"states":    []interface{}{"OPEN"},
		"orderBy":   "CREATED_AT",
		"direction": "DESC",
		"first":     float64(30),
		"after":     (*string)(nil),
	}

	varsClosedOnly := map[string]interface{}{
		"owner":     "owner",
		"repo":      "repo",
		"states":    []interface{}{"CLOSED"},
		"orderBy":   "CREATED_AT",
		"direction": "DESC",
		"first":     float64(30),
		"after":     (*string)(nil),
	}

	varsWithLabels := map[string]interface{}{
		"owner":     "owner",
		"repo":      "repo",
		"states":    []interface{}{"OPEN", "CLOSED"},
		"labels":    []interface{}{"bug", "enhancement"},
		"orderBy":   "CREATED_AT",
		"direction": "DESC",
		"first":     float64(30),
		"after":     (*string)(nil),
	}

	varsRepoNotFound := map[string]interface{}{
		"owner":     "owner",
		"repo":      "nonexistent-repo",
		"states":    []interface{}{"OPEN", "CLOSED"},
		"orderBy":   "CREATED_AT",
		"direction": "DESC",
		"first":     float64(30),
		"after":     (*string)(nil),
	}

	tests := []struct {
		name          string
		reqParams     map[string]interface{}
		expectError   bool
		errContains   string
		expectedCount int
		verifyOrder   func(t *testing.T, issues []*github.Issue)
	}{
		{
			name: "list all issues",
			reqParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
			},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name: "filter by open state",
			reqParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"state": "OPEN",
			},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name: "filter by closed state",
			reqParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"state": "CLOSED",
			},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name: "filter by labels",
			reqParams: map[string]interface{}{
				"owner":  "owner",
				"repo":   "repo",
				"labels": []any{"bug", "enhancement"},
			},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name: "repository not found error",
			reqParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "nonexistent-repo",
			},
			expectError: true,
			errContains: "repository not found",
		},
	}

	// Define the actual query strings that match the implementation
	qBasicNoLabels := "query($after:String$direction:OrderDirection!$first:Int!$orderBy:IssueOrderField!$owner:String!$repo:String!$states:[IssueState!]!){repository(owner: $owner, name: $repo){issues(first: $first, after: $after, states: $states, orderBy: {field: $orderBy, direction: $direction}){nodes{number,title,body,state,databaseId,author{login},createdAt,updatedAt,labels(first: 100){nodes{name,id,description}},comments{totalCount}},pageInfo{hasNextPage,hasPreviousPage,startCursor,endCursor},totalCount}}}"
	qWithLabels := "query($after:String$direction:OrderDirection!$first:Int!$labels:[String!]!$orderBy:IssueOrderField!$owner:String!$repo:String!$states:[IssueState!]!){repository(owner: $owner, name: $repo){issues(first: $first, after: $after, labels: $labels, states: $states, orderBy: {field: $orderBy, direction: $direction}){nodes{number,title,body,state,databaseId,author{login},createdAt,updatedAt,labels(first: 100){nodes{name,id,description}},comments{totalCount}},pageInfo{hasNextPage,hasPreviousPage,startCursor,endCursor},totalCount}}}"

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var httpClient *http.Client

			switch tc.name {
			case "list all issues":
				matcher := githubv4mock.NewQueryMatcher(qBasicNoLabels, varsListAll, mockResponseListAll)
				httpClient = githubv4mock.NewMockedHTTPClient(matcher)
			case "filter by open state":
				matcher := githubv4mock.NewQueryMatcher(qBasicNoLabels, varsOpenOnly, mockResponseOpenOnly)
				httpClient = githubv4mock.NewMockedHTTPClient(matcher)
			case "filter by closed state":
				matcher := githubv4mock.NewQueryMatcher(qBasicNoLabels, varsClosedOnly, mockResponseClosedOnly)
				httpClient = githubv4mock.NewMockedHTTPClient(matcher)
			case "filter by labels":
				matcher := githubv4mock.NewQueryMatcher(qWithLabels, varsWithLabels, mockResponseListAll)
				httpClient = githubv4mock.NewMockedHTTPClient(matcher)
			case "repository not found error":
				matcher := githubv4mock.NewQueryMatcher(qBasicNoLabels, varsRepoNotFound, mockErrorRepoNotFound)
				httpClient = githubv4mock.NewMockedHTTPClient(matcher)
			}

			gqlClient := githubv4.NewClient(httpClient)
			_, handler := ListIssues(stubGetGQLClientFn(gqlClient), translations.NullTranslationHelper)

			req := createMCPRequest(tc.reqParams)
			res, err := handler(context.Background(), req)
			text := getTextResult(t, res).Text

			if tc.expectError {
				require.True(t, res.IsError)
				assert.Contains(t, text, tc.errContains)
				return
			}
			require.NoError(t, err)

			// Parse the structured response with pagination info
			var response struct {
				Issues   []*github.Issue `json:"issues"`
				PageInfo struct {
					HasNextPage     bool   `json:"hasNextPage"`
					HasPreviousPage bool   `json:"hasPreviousPage"`
					StartCursor     string `json:"startCursor"`
					EndCursor       string `json:"endCursor"`
				} `json:"pageInfo"`
				TotalCount int `json:"totalCount"`
			}
			err = json.Unmarshal([]byte(text), &response)
			require.NoError(t, err)

			assert.Len(t, response.Issues, tc.expectedCount, "Expected %d issues, got %d", tc.expectedCount, len(response.Issues))

			// Verify order if verifyOrder function is provided
			if tc.verifyOrder != nil {
				tc.verifyOrder(t, response.Issues)
			}

			// Verify that returned issues have expected structure
			for _, issue := range response.Issues {
				assert.NotNil(t, issue.Number, "Issue should have number")
				assert.NotNil(t, issue.Title, "Issue should have title")
				assert.NotNil(t, issue.State, "Issue should have state")
			}
		})
	}
}

func Test_UpdateIssue(t *testing.T) {
	// Verify tool definition
	mockClient := github.NewClient(nil)
	mockGQLClient := githubv4.NewClient(nil)
	tool, _ := UpdateIssue(stubGetClientFn(mockClient), stubGetGQLClientFn(mockGQLClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "update_issue", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.Contains(t, tool.InputSchema.Properties, "title")
	assert.Contains(t, tool.InputSchema.Properties, "body")
	assert.Contains(t, tool.InputSchema.Properties, "labels")
	assert.Contains(t, tool.InputSchema.Properties, "assignees")
	assert.Contains(t, tool.InputSchema.Properties, "milestone")
	assert.Contains(t, tool.InputSchema.Properties, "type")
	assert.Contains(t, tool.InputSchema.Properties, "state")
	assert.Contains(t, tool.InputSchema.Properties, "state_reason")
	assert.Contains(t, tool.InputSchema.Properties, "duplicate_of")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number"})

	// Mock issues for reuse across test cases
	mockBaseIssue := &github.Issue{
		Number:    github.Ptr(123),
		Title:     github.Ptr("Title"),
		Body:      github.Ptr("Description"),
		State:     github.Ptr("open"),
		HTMLURL:   github.Ptr("https://github.com/owner/repo/issues/123"),
		Assignees: []*github.User{{Login: github.Ptr("assignee1")}, {Login: github.Ptr("assignee2")}},
		Labels:    []*github.Label{{Name: github.Ptr("bug")}, {Name: github.Ptr("priority")}},
		Milestone: &github.Milestone{Number: github.Ptr(5)},
		Type:      &github.IssueType{Name: github.Ptr("Bug")},
	}

	mockUpdatedIssue := &github.Issue{
		Number:      github.Ptr(123),
		Title:       github.Ptr("Updated Title"),
		Body:        github.Ptr("Updated Description"),
		State:       github.Ptr("closed"),
		StateReason: github.Ptr("duplicate"),
		HTMLURL:     github.Ptr("https://github.com/owner/repo/issues/123"),
		Assignees:   []*github.User{{Login: github.Ptr("assignee1")}, {Login: github.Ptr("assignee2")}},
		Labels:      []*github.Label{{Name: github.Ptr("bug")}, {Name: github.Ptr("priority")}},
		Milestone:   &github.Milestone{Number: github.Ptr(5)},
		Type:        &github.IssueType{Name: github.Ptr("Bug")},
	}

	mockReopenedIssue := &github.Issue{
		Number:      github.Ptr(123),
		Title:       github.Ptr("Title"),
		State:       github.Ptr("open"),
		StateReason: github.Ptr("reopened"),
		HTMLURL:     github.Ptr("https://github.com/owner/repo/issues/123"),
	}

	// Mock GraphQL responses for reuse across test cases
	issueIDQueryResponse := githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"issue": map[string]any{
				"id": "I_kwDOA0xdyM50BPaO",
			},
		},
	})

	duplicateIssueIDQueryResponse := githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"issue": map[string]any{
				"id": "I_kwDOA0xdyM50BPaO",
			},
			"duplicateIssue": map[string]any{
				"id": "I_kwDOA0xdyM50BPbP",
			},
		},
	})

	closeSuccessResponse := githubv4mock.DataResponse(map[string]any{
		"closeIssue": map[string]any{
			"issue": map[string]any{
				"id":     "I_kwDOA0xdyM50BPaO",
				"number": 123,
				"url":    "https://github.com/owner/repo/issues/123",
				"state":  "CLOSED",
			},
		},
	})

	reopenSuccessResponse := githubv4mock.DataResponse(map[string]any{
		"reopenIssue": map[string]any{
			"issue": map[string]any{
				"id":     "I_kwDOA0xdyM50BPaO",
				"number": 123,
				"url":    "https://github.com/owner/repo/issues/123",
				"state":  "OPEN",
			},
		},
	})

	duplicateStateReason := IssueClosedStateReasonDuplicate

	tests := []struct {
		name             string
		mockedRESTClient *http.Client
		mockedGQLClient  *http.Client
		requestArgs      map[string]interface{}
		expectError      bool
		expectedIssue    *github.Issue
		expectedErrMsg   string
	}{
		{
			name: "partial update of non-state fields only",
			mockedRESTClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					expectRequestBody(t, map[string]interface{}{
						"title": "Updated Title",
						"body":  "Updated Description",
					}).andThen(
						mockResponse(t, http.StatusOK, mockUpdatedIssue),
					),
				),
			),
			mockedGQLClient: githubv4mock.NewMockedHTTPClient(),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(123),
				"title":        "Updated Title",
				"body":         "Updated Description",
			},
			expectError:   false,
			expectedIssue: mockUpdatedIssue,
		},
		{
			name: "issue not found when updating non-state fields only",
			mockedRESTClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusNotFound)
						_, _ = w.Write([]byte(`{"message": "Not Found"}`))
					}),
				),
			),
			mockedGQLClient: githubv4mock.NewMockedHTTPClient(),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
				"title":        "Updated Title",
			},
			expectError:    true,
			expectedErrMsg: "failed to update issue",
		},
		{
			name: "close issue as duplicate",
			mockedRESTClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					mockBaseIssue,
				),
			),
			mockedGQLClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID githubv4.ID
							} `graphql:"issue(number: $issueNumber)"`
							DuplicateIssue struct {
								ID githubv4.ID
							} `graphql:"duplicateIssue: issue(number: $duplicateOf)"`
						} `graphql:"repository(owner: $owner, name: $repo)"`
					}{},
					map[string]any{
						"owner":       githubv4.String("owner"),
						"repo":        githubv4.String("repo"),
						"issueNumber": githubv4.Int(123),
						"duplicateOf": githubv4.Int(456),
					},
					duplicateIssueIDQueryResponse,
				),
				githubv4mock.NewMutationMatcher(
					struct {
						CloseIssue struct {
							Issue struct {
								ID     githubv4.ID
								Number githubv4.Int
								URL    githubv4.String
								State  githubv4.String
							}
						} `graphql:"closeIssue(input: $input)"`
					}{},
					CloseIssueInput{
						IssueID:          "I_kwDOA0xdyM50BPaO",
						StateReason:      &duplicateStateReason,
						DuplicateIssueID: githubv4.NewID("I_kwDOA0xdyM50BPbP"),
					},
					nil,
					closeSuccessResponse,
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(123),
				"state":        "closed",
				"state_reason": "duplicate",
				"duplicate_of": float64(456),
			},
			expectError:   false,
			expectedIssue: mockUpdatedIssue,
		},
		{
			name: "reopen issue",
			mockedRESTClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					mockBaseIssue,
				),
			),
			mockedGQLClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID githubv4.ID
							} `graphql:"issue(number: $issueNumber)"`
						} `graphql:"repository(owner: $owner, name: $repo)"`
					}{},
					map[string]any{
						"owner":       githubv4.String("owner"),
						"repo":        githubv4.String("repo"),
						"issueNumber": githubv4.Int(123),
					},
					issueIDQueryResponse,
				),
				githubv4mock.NewMutationMatcher(
					struct {
						ReopenIssue struct {
							Issue struct {
								ID     githubv4.ID
								Number githubv4.Int
								URL    githubv4.String
								State  githubv4.String
							}
						} `graphql:"reopenIssue(input: $input)"`
					}{},
					githubv4.ReopenIssueInput{
						IssueID: "I_kwDOA0xdyM50BPaO",
					},
					nil,
					reopenSuccessResponse,
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(123),
				"state":        "open",
			},
			expectError:   false,
			expectedIssue: mockReopenedIssue,
		},
		{
			name: "main issue not found when trying to close it",
			mockedRESTClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					mockBaseIssue,
				),
			),
			mockedGQLClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID githubv4.ID
							} `graphql:"issue(number: $issueNumber)"`
						} `graphql:"repository(owner: $owner, name: $repo)"`
					}{},
					map[string]any{
						"owner":       githubv4.String("owner"),
						"repo":        githubv4.String("repo"),
						"issueNumber": githubv4.Int(999),
					},
					githubv4mock.ErrorResponse("Could not resolve to an Issue with the number of 999."),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
				"state":        "closed",
				"state_reason": "not_planned",
			},
			expectError:    true,
			expectedErrMsg: "Failed to find issues",
		},
		{
			name: "duplicate issue not found when closing as duplicate",
			mockedRESTClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					mockBaseIssue,
				),
			),
			mockedGQLClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID githubv4.ID
							} `graphql:"issue(number: $issueNumber)"`
							DuplicateIssue struct {
								ID githubv4.ID
							} `graphql:"duplicateIssue: issue(number: $duplicateOf)"`
						} `graphql:"repository(owner: $owner, name: $repo)"`
					}{},
					map[string]any{
						"owner":       githubv4.String("owner"),
						"repo":        githubv4.String("repo"),
						"issueNumber": githubv4.Int(123),
						"duplicateOf": githubv4.Int(999),
					},
					githubv4mock.ErrorResponse("Could not resolve to an Issue with the number of 999."),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(123),
				"state":        "closed",
				"state_reason": "duplicate",
				"duplicate_of": float64(999),
			},
			expectError:    true,
			expectedErrMsg: "Failed to find issues",
		},
		{
			name: "close as duplicate with combined non-state updates",
			mockedRESTClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					expectRequestBody(t, map[string]interface{}{
						"title":     "Updated Title",
						"body":      "Updated Description",
						"labels":    []any{"bug", "priority"},
						"assignees": []any{"assignee1", "assignee2"},
						"milestone": float64(5),
						"type":      "Bug",
					}).andThen(
						mockResponse(t, http.StatusOK, &github.Issue{
							Number:    github.Ptr(123),
							Title:     github.Ptr("Updated Title"),
							Body:      github.Ptr("Updated Description"),
							Labels:    []*github.Label{{Name: github.Ptr("bug")}, {Name: github.Ptr("priority")}},
							Assignees: []*github.User{{Login: github.Ptr("assignee1")}, {Login: github.Ptr("assignee2")}},
							Milestone: &github.Milestone{Number: github.Ptr(5)},
							Type:      &github.IssueType{Name: github.Ptr("Bug")},
							State:     github.Ptr("open"), // Still open after REST update
							HTMLURL:   github.Ptr("https://github.com/owner/repo/issues/123"),
						}),
					),
				),
			),
			mockedGQLClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID githubv4.ID
							} `graphql:"issue(number: $issueNumber)"`
							DuplicateIssue struct {
								ID githubv4.ID
							} `graphql:"duplicateIssue: issue(number: $duplicateOf)"`
						} `graphql:"repository(owner: $owner, name: $repo)"`
					}{},
					map[string]any{
						"owner":       githubv4.String("owner"),
						"repo":        githubv4.String("repo"),
						"issueNumber": githubv4.Int(123),
						"duplicateOf": githubv4.Int(456),
					},
					duplicateIssueIDQueryResponse,
				),
				githubv4mock.NewMutationMatcher(
					struct {
						CloseIssue struct {
							Issue struct {
								ID     githubv4.ID
								Number githubv4.Int
								URL    githubv4.String
								State  githubv4.String
							}
						} `graphql:"closeIssue(input: $input)"`
					}{},
					CloseIssueInput{
						IssueID:          "I_kwDOA0xdyM50BPaO",
						StateReason:      &duplicateStateReason,
						DuplicateIssueID: githubv4.NewID("I_kwDOA0xdyM50BPbP"),
					},
					nil,
					closeSuccessResponse,
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(123),
				"title":        "Updated Title",
				"body":         "Updated Description",
				"labels":       []any{"bug", "priority"},
				"assignees":    []any{"assignee1", "assignee2"},
				"milestone":    float64(5),
				"type":         "Bug",
				"state":        "closed",
				"state_reason": "duplicate",
				"duplicate_of": float64(456),
			},
			expectError:   false,
			expectedIssue: mockUpdatedIssue,
		},
		{
			name:             "duplicate_of without duplicate state_reason should fail",
			mockedRESTClient: mock.NewMockedHTTPClient(),
			mockedGQLClient:  githubv4mock.NewMockedHTTPClient(),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(123),
				"state":        "closed",
				"state_reason": "completed",
				"duplicate_of": float64(456),
			},
			expectError:    true,
			expectedErrMsg: "duplicate_of can only be used when state_reason is 'duplicate'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup clients with mocks
			restClient := github.NewClient(tc.mockedRESTClient)
			gqlClient := githubv4.NewClient(tc.mockedGQLClient)
			_, handler := UpdateIssue(stubGetClientFn(restClient), stubGetGQLClientFn(gqlClient), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError || tc.expectedErrMsg != "" {
				require.NoError(t, err)
				require.True(t, result.IsError)
				errorContent := getErrorResult(t, result)
				if tc.expectedErrMsg != "" {
					assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				}
				return
			}

			require.NoError(t, err)
			require.False(t, result.IsError)

			// Parse the result and get the text content
			textContent := getTextResult(t, result)

			// Unmarshal and verify the minimal result
			var updateResp MinimalResponse
			err = json.Unmarshal([]byte(textContent.Text), &updateResp)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedIssue.GetHTMLURL(), updateResp.URL)
		})
	}
}

func Test_ParseISOTimestamp(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedErr  bool
		expectedTime time.Time
	}{
		{
			name:         "valid RFC3339 format",
			input:        "2023-01-15T14:30:00Z",
			expectedErr:  false,
			expectedTime: time.Date(2023, 1, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:         "valid date only format",
			input:        "2023-01-15",
			expectedErr:  false,
			expectedTime: time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "empty timestamp",
			input:       "",
			expectedErr: true,
		},
		{
			name:        "invalid format",
			input:       "15/01/2023",
			expectedErr: true,
		},
		{
			name:        "invalid date",
			input:       "2023-13-45",
			expectedErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsedTime, err := parseISOTimestamp(tc.input)

			if tc.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedTime, parsedTime)
			}
		})
	}
}

func Test_GetIssueComments(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := GetIssueComments(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "get_issue_comments", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.Contains(t, tool.InputSchema.Properties, "page")
	assert.Contains(t, tool.InputSchema.Properties, "perPage")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number"})

	// Setup mock comments for success case
	mockComments := []*github.IssueComment{
		{
			ID:   github.Ptr(int64(123)),
			Body: github.Ptr("This is the first comment"),
			User: &github.User{
				Login: github.Ptr("user1"),
			},
			CreatedAt: &github.Timestamp{Time: time.Now().Add(-time.Hour * 24)},
		},
		{
			ID:   github.Ptr(int64(456)),
			Body: github.Ptr("This is the second comment"),
			User: &github.User{
				Login: github.Ptr("user2"),
			},
			CreatedAt: &github.Timestamp{Time: time.Now().Add(-time.Hour)},
		},
	}

	tests := []struct {
		name             string
		mockedClient     *http.Client
		requestArgs      map[string]interface{}
		expectError      bool
		expectedComments []*github.IssueComment
		expectedErrMsg   string
	}{
		{
			name: "successful comments retrieval",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposIssuesCommentsByOwnerByRepoByIssueNumber,
					mockComments,
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:      false,
			expectedComments: mockComments,
		},
		{
			name: "successful comments retrieval with pagination",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesCommentsByOwnerByRepoByIssueNumber,
					expectQueryParams(t, map[string]string{
						"page":     "2",
						"per_page": "10",
					}).andThen(
						mockResponse(t, http.StatusOK, mockComments),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"page":         float64(2),
				"perPage":      float64(10),
			},
			expectError:      false,
			expectedComments: mockComments,
		},
		{
			name: "issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesCommentsByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Issue not found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
			},
			expectError:    true,
			expectedErrMsg: "failed to get issue comments",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := GetIssueComments(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedComments []*github.IssueComment
			err = json.Unmarshal([]byte(textContent.Text), &returnedComments)
			require.NoError(t, err)
			assert.Equal(t, len(tc.expectedComments), len(returnedComments))
			if len(returnedComments) > 0 {
				assert.Equal(t, *tc.expectedComments[0].Body, *returnedComments[0].Body)
				assert.Equal(t, *tc.expectedComments[0].User.Login, *returnedComments[0].User.Login)
			}
		})
	}
}

func TestAssignCopilotToIssue(t *testing.T) {
	t.Parallel()

	// Verify tool definition
	mockClient := githubv4.NewClient(nil)
	tool, _ := AssignCopilotToIssue(stubGetGQLClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "assign_copilot_to_issue", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issueNumber")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issueNumber"})

	var pageOfFakeBots = func(n int) []struct{} {
		// We don't _really_ need real bots here, just objects that count as entries for the page
		bots := make([]struct{}, n)
		for i := range n {
			bots[i] = struct{}{}
		}
		return bots
	}

	tests := []struct {
		name               string
		requestArgs        map[string]any
		mockedClient       *http.Client
		expectToolError    bool
		expectedToolErrMsg string
	}{
		{
			name: "successful assignment when there are no existing assignees",
			requestArgs: map[string]any{
				"owner":       "owner",
				"repo":        "repo",
				"issueNumber": float64(123),
			},
			mockedClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							SuggestedActors struct {
								Nodes []struct {
									Bot struct {
										ID       githubv4.ID
										Login    githubv4.String
										TypeName string `graphql:"__typename"`
									} `graphql:"... on Bot"`
								}
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
							} `graphql:"suggestedActors(first: 100, after: $endCursor, capabilities: CAN_BE_ASSIGNED)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":     githubv4.String("owner"),
						"name":      githubv4.String("repo"),
						"endCursor": (*githubv4.String)(nil),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"suggestedActors": map[string]any{
								"nodes": []any{
									map[string]any{
										"id":         githubv4.ID("copilot-swe-agent-id"),
										"login":      githubv4.String("copilot-swe-agent"),
										"__typename": "Bot",
									},
								},
							},
						},
					}),
				),
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID        githubv4.ID
								Assignees struct {
									Nodes []struct {
										ID githubv4.ID
									}
								} `graphql:"assignees(first: 100)"`
							} `graphql:"issue(number: $number)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":  githubv4.String("owner"),
						"name":   githubv4.String("repo"),
						"number": githubv4.Int(123),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"issue": map[string]any{
								"id": githubv4.ID("test-issue-id"),
								"assignees": map[string]any{
									"nodes": []any{},
								},
							},
						},
					}),
				),
				githubv4mock.NewMutationMatcher(
					struct {
						ReplaceActorsForAssignable struct {
							Typename string `graphql:"__typename"`
						} `graphql:"replaceActorsForAssignable(input: $input)"`
					}{},
					ReplaceActorsForAssignableInput{
						AssignableID: githubv4.ID("test-issue-id"),
						ActorIDs:     []githubv4.ID{githubv4.ID("copilot-swe-agent-id")},
					},
					nil,
					githubv4mock.DataResponse(map[string]any{}),
				),
			),
		},
		{
			name: "successful assignment when there are existing assignees",
			requestArgs: map[string]any{
				"owner":       "owner",
				"repo":        "repo",
				"issueNumber": float64(123),
			},
			mockedClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							SuggestedActors struct {
								Nodes []struct {
									Bot struct {
										ID       githubv4.ID
										Login    githubv4.String
										TypeName string `graphql:"__typename"`
									} `graphql:"... on Bot"`
								}
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
							} `graphql:"suggestedActors(first: 100, after: $endCursor, capabilities: CAN_BE_ASSIGNED)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":     githubv4.String("owner"),
						"name":      githubv4.String("repo"),
						"endCursor": (*githubv4.String)(nil),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"suggestedActors": map[string]any{
								"nodes": []any{
									map[string]any{
										"id":         githubv4.ID("copilot-swe-agent-id"),
										"login":      githubv4.String("copilot-swe-agent"),
										"__typename": "Bot",
									},
								},
							},
						},
					}),
				),
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID        githubv4.ID
								Assignees struct {
									Nodes []struct {
										ID githubv4.ID
									}
								} `graphql:"assignees(first: 100)"`
							} `graphql:"issue(number: $number)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":  githubv4.String("owner"),
						"name":   githubv4.String("repo"),
						"number": githubv4.Int(123),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"issue": map[string]any{
								"id": githubv4.ID("test-issue-id"),
								"assignees": map[string]any{
									"nodes": []any{
										map[string]any{
											"id": githubv4.ID("existing-assignee-id"),
										},
										map[string]any{
											"id": githubv4.ID("existing-assignee-id-2"),
										},
									},
								},
							},
						},
					}),
				),
				githubv4mock.NewMutationMatcher(
					struct {
						ReplaceActorsForAssignable struct {
							Typename string `graphql:"__typename"`
						} `graphql:"replaceActorsForAssignable(input: $input)"`
					}{},
					ReplaceActorsForAssignableInput{
						AssignableID: githubv4.ID("test-issue-id"),
						ActorIDs: []githubv4.ID{
							githubv4.ID("existing-assignee-id"),
							githubv4.ID("existing-assignee-id-2"),
							githubv4.ID("copilot-swe-agent-id"),
						},
					},
					nil,
					githubv4mock.DataResponse(map[string]any{}),
				),
			),
		},
		{
			name: "copilot bot not on first page of suggested actors",
			requestArgs: map[string]any{
				"owner":       "owner",
				"repo":        "repo",
				"issueNumber": float64(123),
			},
			mockedClient: githubv4mock.NewMockedHTTPClient(
				// First page of suggested actors
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							SuggestedActors struct {
								Nodes []struct {
									Bot struct {
										ID       githubv4.ID
										Login    githubv4.String
										TypeName string `graphql:"__typename"`
									} `graphql:"... on Bot"`
								}
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
							} `graphql:"suggestedActors(first: 100, after: $endCursor, capabilities: CAN_BE_ASSIGNED)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":     githubv4.String("owner"),
						"name":      githubv4.String("repo"),
						"endCursor": (*githubv4.String)(nil),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"suggestedActors": map[string]any{
								"nodes": pageOfFakeBots(100),
								"pageInfo": map[string]any{
									"hasNextPage": true,
									"endCursor":   githubv4.String("next-page-cursor"),
								},
							},
						},
					}),
				),
				// Second page of suggested actors
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							SuggestedActors struct {
								Nodes []struct {
									Bot struct {
										ID       githubv4.ID
										Login    githubv4.String
										TypeName string `graphql:"__typename"`
									} `graphql:"... on Bot"`
								}
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
							} `graphql:"suggestedActors(first: 100, after: $endCursor, capabilities: CAN_BE_ASSIGNED)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":     githubv4.String("owner"),
						"name":      githubv4.String("repo"),
						"endCursor": githubv4.String("next-page-cursor"),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"suggestedActors": map[string]any{
								"nodes": []any{
									map[string]any{
										"id":         githubv4.ID("copilot-swe-agent-id"),
										"login":      githubv4.String("copilot-swe-agent"),
										"__typename": "Bot",
									},
								},
							},
						},
					}),
				),
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							Issue struct {
								ID        githubv4.ID
								Assignees struct {
									Nodes []struct {
										ID githubv4.ID
									}
								} `graphql:"assignees(first: 100)"`
							} `graphql:"issue(number: $number)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":  githubv4.String("owner"),
						"name":   githubv4.String("repo"),
						"number": githubv4.Int(123),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"issue": map[string]any{
								"id": githubv4.ID("test-issue-id"),
								"assignees": map[string]any{
									"nodes": []any{},
								},
							},
						},
					}),
				),
				githubv4mock.NewMutationMatcher(
					struct {
						ReplaceActorsForAssignable struct {
							Typename string `graphql:"__typename"`
						} `graphql:"replaceActorsForAssignable(input: $input)"`
					}{},
					ReplaceActorsForAssignableInput{
						AssignableID: githubv4.ID("test-issue-id"),
						ActorIDs:     []githubv4.ID{githubv4.ID("copilot-swe-agent-id")},
					},
					nil,
					githubv4mock.DataResponse(map[string]any{}),
				),
			),
		},
		{
			name: "copilot not a suggested actor",
			requestArgs: map[string]any{
				"owner":       "owner",
				"repo":        "repo",
				"issueNumber": float64(123),
			},
			mockedClient: githubv4mock.NewMockedHTTPClient(
				githubv4mock.NewQueryMatcher(
					struct {
						Repository struct {
							SuggestedActors struct {
								Nodes []struct {
									Bot struct {
										ID       githubv4.ID
										Login    githubv4.String
										TypeName string `graphql:"__typename"`
									} `graphql:"... on Bot"`
								}
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
							} `graphql:"suggestedActors(first: 100, after: $endCursor, capabilities: CAN_BE_ASSIGNED)"`
						} `graphql:"repository(owner: $owner, name: $name)"`
					}{},
					map[string]any{
						"owner":     githubv4.String("owner"),
						"name":      githubv4.String("repo"),
						"endCursor": (*githubv4.String)(nil),
					},
					githubv4mock.DataResponse(map[string]any{
						"repository": map[string]any{
							"suggestedActors": map[string]any{
								"nodes": []any{},
							},
						},
					}),
				),
			),
			expectToolError:    true,
			expectedToolErrMsg: "copilot isn't available as an assignee for this issue. Please inform the user to visit https://docs.github.com/en/copilot/using-github-copilot/using-copilot-coding-agent-to-work-on-tasks/about-assigning-tasks-to-copilot for more information.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			t.Parallel()
			// Setup client with mock
			client := githubv4.NewClient(tc.mockedClient)
			_, handler := AssignCopilotToIssue(stubGetGQLClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)
			require.NoError(t, err)

			textContent := getTextResult(t, result)

			if tc.expectToolError {
				require.True(t, result.IsError)
				assert.Contains(t, textContent.Text, tc.expectedToolErrMsg)
				return
			}

			require.False(t, result.IsError, fmt.Sprintf("expected there to be no tool error, text was %s", textContent.Text))
			require.Equal(t, textContent.Text, "successfully assigned copilot to issue")
		})
	}
}

func Test_AddSubIssue(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := AddSubIssue(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "add_sub_issue", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.Contains(t, tool.InputSchema.Properties, "sub_issue_id")
	assert.Contains(t, tool.InputSchema.Properties, "replace_parent")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number", "sub_issue_id"})

	// Setup mock issue for success case (matches GitHub API response format)
	mockIssue := &github.Issue{
		Number:  github.Ptr(42),
		Title:   github.Ptr("Parent Issue"),
		Body:    github.Ptr("This is the parent issue with a sub-issue"),
		State:   github.Ptr("open"),
		HTMLURL: github.Ptr("https://github.com/owner/repo/issues/42"),
		User: &github.User{
			Login: github.Ptr("testuser"),
		},
		Labels: []*github.Label{
			{
				Name:        github.Ptr("enhancement"),
				Color:       github.Ptr("84b6eb"),
				Description: github.Ptr("New feature or request"),
			},
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedIssue  *github.Issue
		expectedErrMsg string
	}{
		{
			name: "successful sub-issue addition with all parameters",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusCreated, mockIssue),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":          "owner",
				"repo":           "repo",
				"issue_number":   float64(42),
				"sub_issue_id":   float64(123),
				"replace_parent": true,
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name: "successful sub-issue addition with minimal parameters",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusCreated, mockIssue),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(456),
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name: "successful sub-issue addition with replace_parent false",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusCreated, mockIssue),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":          "owner",
				"repo":           "repo",
				"issue_number":   float64(42),
				"sub_issue_id":   float64(789),
				"replace_parent": false,
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name: "parent issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Parent issue not found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "failed to add sub-issue",
		},
		{
			name: "sub-issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Sub-issue not found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(999),
			},
			expectError:    false,
			expectedErrMsg: "failed to add sub-issue",
		},
		{
			name: "validation failed - sub-issue cannot be parent of itself",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusUnprocessableEntity, `{"message": "Validation failed", "errors": [{"message": "Sub-issue cannot be a parent of itself"}]}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(42),
			},
			expectError:    false,
			expectedErrMsg: "failed to add sub-issue",
		},
		{
			name: "insufficient permissions",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusForbidden, `{"message": "Must have write access to repository"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "failed to add sub-issue",
		},
		{
			name:         "missing required parameter owner",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: owner",
		},
		{
			name:         "missing required parameter sub_issue_id",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: sub_issue_id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := AddSubIssue(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			if tc.expectedErrMsg != "" {
				require.NotNil(t, result)
				textContent := getTextResult(t, result)
				assert.Contains(t, textContent.Text, tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)

			// Parse the result and get the text content if no error
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedIssue github.Issue
			err = json.Unmarshal([]byte(textContent.Text), &returnedIssue)
			require.NoError(t, err)
			assert.Equal(t, *tc.expectedIssue.Number, *returnedIssue.Number)
			assert.Equal(t, *tc.expectedIssue.Title, *returnedIssue.Title)
			assert.Equal(t, *tc.expectedIssue.Body, *returnedIssue.Body)
			assert.Equal(t, *tc.expectedIssue.State, *returnedIssue.State)
			assert.Equal(t, *tc.expectedIssue.HTMLURL, *returnedIssue.HTMLURL)
			assert.Equal(t, *tc.expectedIssue.User.Login, *returnedIssue.User.Login)
		})
	}
}

func Test_ListSubIssues(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := ListSubIssues(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "list_sub_issues", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.Contains(t, tool.InputSchema.Properties, "page")
	assert.Contains(t, tool.InputSchema.Properties, "per_page")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number"})

	// Setup mock sub-issues for success case
	mockSubIssues := []*github.Issue{
		{
			Number:  github.Ptr(123),
			Title:   github.Ptr("Sub-issue 1"),
			Body:    github.Ptr("This is the first sub-issue"),
			State:   github.Ptr("open"),
			HTMLURL: github.Ptr("https://github.com/owner/repo/issues/123"),
			User: &github.User{
				Login: github.Ptr("user1"),
			},
			Labels: []*github.Label{
				{
					Name:        github.Ptr("bug"),
					Color:       github.Ptr("d73a4a"),
					Description: github.Ptr("Something isn't working"),
				},
			},
		},
		{
			Number:  github.Ptr(124),
			Title:   github.Ptr("Sub-issue 2"),
			Body:    github.Ptr("This is the second sub-issue"),
			State:   github.Ptr("closed"),
			HTMLURL: github.Ptr("https://github.com/owner/repo/issues/124"),
			User: &github.User{
				Login: github.Ptr("user2"),
			},
			Assignees: []*github.User{
				{Login: github.Ptr("assignee1")},
			},
		},
	}

	tests := []struct {
		name              string
		mockedClient      *http.Client
		requestArgs       map[string]interface{}
		expectError       bool
		expectedSubIssues []*github.Issue
		expectedErrMsg    string
	}{
		{
			name: "successful sub-issues listing with minimal parameters",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockSubIssues,
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:       false,
			expectedSubIssues: mockSubIssues,
		},
		{
			name: "successful sub-issues listing with pagination",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					expectQueryParams(t, map[string]string{
						"page":     "2",
						"per_page": "10",
					}).andThen(
						mockResponse(t, http.StatusOK, mockSubIssues),
					),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"page":         float64(2),
				"per_page":     float64(10),
			},
			expectError:       false,
			expectedSubIssues: mockSubIssues,
		},
		{
			name: "successful sub-issues listing with empty result",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					[]*github.Issue{},
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:       false,
			expectedSubIssues: []*github.Issue{},
		},
		{
			name: "parent issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Not Found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
			},
			expectError:    false,
			expectedErrMsg: "failed to list sub-issues",
		},
		{
			name: "repository not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Not Found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "nonexistent",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:    false,
			expectedErrMsg: "failed to list sub-issues",
		},
		{
			name: "sub-issues feature gone/deprecated",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesSubIssuesByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusGone, `{"message": "This feature has been deprecated"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:    false,
			expectedErrMsg: "failed to list sub-issues",
		},
		{
			name:         "missing required parameter owner",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: owner",
		},
		{
			name:         "missing required parameter issue_number",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: issue_number",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := ListSubIssues(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			if tc.expectedErrMsg != "" {
				require.NotNil(t, result)
				textContent := getTextResult(t, result)
				assert.Contains(t, textContent.Text, tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)

			// Parse the result and get the text content if no error
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedSubIssues []*github.Issue
			err = json.Unmarshal([]byte(textContent.Text), &returnedSubIssues)
			require.NoError(t, err)

			assert.Len(t, returnedSubIssues, len(tc.expectedSubIssues))
			for i, subIssue := range returnedSubIssues {
				if i < len(tc.expectedSubIssues) {
					assert.Equal(t, *tc.expectedSubIssues[i].Number, *subIssue.Number)
					assert.Equal(t, *tc.expectedSubIssues[i].Title, *subIssue.Title)
					assert.Equal(t, *tc.expectedSubIssues[i].State, *subIssue.State)
					assert.Equal(t, *tc.expectedSubIssues[i].HTMLURL, *subIssue.HTMLURL)
					assert.Equal(t, *tc.expectedSubIssues[i].User.Login, *subIssue.User.Login)

					if tc.expectedSubIssues[i].Body != nil {
						assert.Equal(t, *tc.expectedSubIssues[i].Body, *subIssue.Body)
					}
				}
			}
		})
	}
}

func Test_RemoveSubIssue(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := RemoveSubIssue(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "remove_sub_issue", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.Contains(t, tool.InputSchema.Properties, "sub_issue_id")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number", "sub_issue_id"})

	// Setup mock issue for success case (matches GitHub API response format - the updated parent issue)
	mockIssue := &github.Issue{
		Number:  github.Ptr(42),
		Title:   github.Ptr("Parent Issue"),
		Body:    github.Ptr("This is the parent issue after sub-issue removal"),
		State:   github.Ptr("open"),
		HTMLURL: github.Ptr("https://github.com/owner/repo/issues/42"),
		User: &github.User{
			Login: github.Ptr("testuser"),
		},
		Labels: []*github.Label{
			{
				Name:        github.Ptr("enhancement"),
				Color:       github.Ptr("84b6eb"),
				Description: github.Ptr("New feature or request"),
			},
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedIssue  *github.Issue
		expectedErrMsg string
	}{
		{
			name: "successful sub-issue removal",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.DeleteReposIssuesSubIssueByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusOK, mockIssue),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name: "parent issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.DeleteReposIssuesSubIssueByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Not Found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "failed to remove sub-issue",
		},
		{
			name: "sub-issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.DeleteReposIssuesSubIssueByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Sub-issue not found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(999),
			},
			expectError:    false,
			expectedErrMsg: "failed to remove sub-issue",
		},
		{
			name: "bad request - invalid sub_issue_id",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.DeleteReposIssuesSubIssueByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusBadRequest, `{"message": "Invalid sub_issue_id"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(-1),
			},
			expectError:    false,
			expectedErrMsg: "failed to remove sub-issue",
		},
		{
			name: "repository not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.DeleteReposIssuesSubIssueByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Not Found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "nonexistent",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "failed to remove sub-issue",
		},
		{
			name: "insufficient permissions",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.DeleteReposIssuesSubIssueByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusForbidden, `{"message": "Must have write access to repository"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "failed to remove sub-issue",
		},
		{
			name:         "missing required parameter owner",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: owner",
		},
		{
			name:         "missing required parameter sub_issue_id",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: sub_issue_id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := RemoveSubIssue(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			if tc.expectedErrMsg != "" {
				require.NotNil(t, result)
				textContent := getTextResult(t, result)
				assert.Contains(t, textContent.Text, tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)

			// Parse the result and get the text content if no error
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedIssue github.Issue
			err = json.Unmarshal([]byte(textContent.Text), &returnedIssue)
			require.NoError(t, err)
			assert.Equal(t, *tc.expectedIssue.Number, *returnedIssue.Number)
			assert.Equal(t, *tc.expectedIssue.Title, *returnedIssue.Title)
			assert.Equal(t, *tc.expectedIssue.Body, *returnedIssue.Body)
			assert.Equal(t, *tc.expectedIssue.State, *returnedIssue.State)
			assert.Equal(t, *tc.expectedIssue.HTMLURL, *returnedIssue.HTMLURL)
			assert.Equal(t, *tc.expectedIssue.User.Login, *returnedIssue.User.Login)
		})
	}
}

func Test_ReprioritizeSubIssue(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := ReprioritizeSubIssue(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "reprioritize_sub_issue", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "issue_number")
	assert.Contains(t, tool.InputSchema.Properties, "sub_issue_id")
	assert.Contains(t, tool.InputSchema.Properties, "after_id")
	assert.Contains(t, tool.InputSchema.Properties, "before_id")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "issue_number", "sub_issue_id"})

	// Setup mock issue for success case (matches GitHub API response format - the updated parent issue)
	mockIssue := &github.Issue{
		Number:  github.Ptr(42),
		Title:   github.Ptr("Parent Issue"),
		Body:    github.Ptr("This is the parent issue with reprioritized sub-issues"),
		State:   github.Ptr("open"),
		HTMLURL: github.Ptr("https://github.com/owner/repo/issues/42"),
		User: &github.User{
			Login: github.Ptr("testuser"),
		},
		Labels: []*github.Label{
			{
				Name:        github.Ptr("enhancement"),
				Color:       github.Ptr("84b6eb"),
				Description: github.Ptr("New feature or request"),
			},
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedIssue  *github.Issue
		expectedErrMsg string
	}{
		{
			name: "successful reprioritization with after_id",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesSubIssuesPriorityByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusOK, mockIssue),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
				"after_id":     float64(456),
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name: "successful reprioritization with before_id",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesSubIssuesPriorityByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusOK, mockIssue),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
				"before_id":    float64(789),
			},
			expectError:   false,
			expectedIssue: mockIssue,
		},
		{
			name:         "validation error - neither after_id nor before_id specified",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
			},
			expectError:    false,
			expectedErrMsg: "either after_id or before_id must be specified",
		},
		{
			name:         "validation error - both after_id and before_id specified",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
				"after_id":     float64(456),
				"before_id":    float64(789),
			},
			expectError:    false,
			expectedErrMsg: "only one of after_id or before_id should be specified, not both",
		},
		{
			name: "parent issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesSubIssuesPriorityByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Not Found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(999),
				"sub_issue_id": float64(123),
				"after_id":     float64(456),
			},
			expectError:    false,
			expectedErrMsg: "failed to reprioritize sub-issue",
		},
		{
			name: "sub-issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesSubIssuesPriorityByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusNotFound, `{"message": "Sub-issue not found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(999),
				"after_id":     float64(456),
			},
			expectError:    false,
			expectedErrMsg: "failed to reprioritize sub-issue",
		},
		{
			name: "validation failed - positioning sub-issue not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesSubIssuesPriorityByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusUnprocessableEntity, `{"message": "Validation failed", "errors": [{"message": "Positioning sub-issue not found"}]}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
				"after_id":     float64(999),
			},
			expectError:    false,
			expectedErrMsg: "failed to reprioritize sub-issue",
		},
		{
			name: "insufficient permissions",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesSubIssuesPriorityByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusForbidden, `{"message": "Must have write access to repository"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
				"after_id":     float64(456),
			},
			expectError:    false,
			expectedErrMsg: "failed to reprioritize sub-issue",
		},
		{
			name: "service unavailable",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesSubIssuesPriorityByOwnerByRepoByIssueNumber,
					mockResponse(t, http.StatusServiceUnavailable, `{"message": "Service Unavailable"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
				"before_id":    float64(456),
			},
			expectError:    false,
			expectedErrMsg: "failed to reprioritize sub-issue",
		},
		{
			name:         "missing required parameter owner",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"repo":         "repo",
				"issue_number": float64(42),
				"sub_issue_id": float64(123),
				"after_id":     float64(456),
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: owner",
		},
		{
			name:         "missing required parameter sub_issue_id",
			mockedClient: mock.NewMockedHTTPClient(
			// No mocked requests needed since validation fails before HTTP call
			),
			requestArgs: map[string]interface{}{
				"owner":        "owner",
				"repo":         "repo",
				"issue_number": float64(42),
				"after_id":     float64(456),
			},
			expectError:    false,
			expectedErrMsg: "missing required parameter: sub_issue_id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := ReprioritizeSubIssue(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			if tc.expectedErrMsg != "" {
				require.NotNil(t, result)
				textContent := getTextResult(t, result)
				assert.Contains(t, textContent.Text, tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)

			// Parse the result and get the text content if no error
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedIssue github.Issue
			err = json.Unmarshal([]byte(textContent.Text), &returnedIssue)
			require.NoError(t, err)
			assert.Equal(t, *tc.expectedIssue.Number, *returnedIssue.Number)
			assert.Equal(t, *tc.expectedIssue.Title, *returnedIssue.Title)
			assert.Equal(t, *tc.expectedIssue.Body, *returnedIssue.Body)
			assert.Equal(t, *tc.expectedIssue.State, *returnedIssue.State)
			assert.Equal(t, *tc.expectedIssue.HTMLURL, *returnedIssue.HTMLURL)
			assert.Equal(t, *tc.expectedIssue.User.Login, *returnedIssue.User.Login)
		})
	}
}

func Test_ListIssueTypes(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	tool, _ := ListIssueTypes(stubGetClientFn(mockClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "list_issue_types", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner"})

	// Setup mock issue types for success case
	mockIssueTypes := []*github.IssueType{
		{
			ID:          github.Ptr(int64(1)),
			Name:        github.Ptr("bug"),
			Description: github.Ptr("Something isn't working"),
			Color:       github.Ptr("d73a4a"),
		},
		{
			ID:          github.Ptr(int64(2)),
			Name:        github.Ptr("feature"),
			Description: github.Ptr("New feature or enhancement"),
			Color:       github.Ptr("a2eeef"),
		},
	}

	tests := []struct {
		name               string
		mockedClient       *http.Client
		requestArgs        map[string]interface{}
		expectError        bool
		expectedIssueTypes []*github.IssueType
		expectedErrMsg     string
	}{
		{
			name: "successful issue types retrieval",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Pattern: "/orgs/testorg/issue-types",
						Method:  "GET",
					},
					mockResponse(t, http.StatusOK, mockIssueTypes),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "testorg",
			},
			expectError:        false,
			expectedIssueTypes: mockIssueTypes,
		},
		{
			name: "organization not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Pattern: "/orgs/nonexistent/issue-types",
						Method:  "GET",
					},
					mockResponse(t, http.StatusNotFound, `{"message": "Organization not found"}`),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "nonexistent",
			},
			expectError:    true,
			expectedErrMsg: "failed to list issue types",
		},
		{
			name: "missing owner parameter",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Pattern: "/orgs/testorg/issue-types",
						Method:  "GET",
					},
					mockResponse(t, http.StatusOK, mockIssueTypes),
				),
			),
			requestArgs:    map[string]interface{}{},
			expectError:    false, // This should be handled by parameter validation, error returned in result
			expectedErrMsg: "missing required parameter: owner",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := ListIssueTypes(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				if err != nil {
					assert.Contains(t, err.Error(), tc.expectedErrMsg)
					return
				}
				// Check if error is returned as tool result error
				require.NotNil(t, result)
				require.True(t, result.IsError)
				errorContent := getErrorResult(t, result)
				assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				return
			}

			// Check if it's a parameter validation error (returned as tool result error)
			if result != nil && result.IsError {
				errorContent := getErrorResult(t, result)
				if tc.expectedErrMsg != "" && strings.Contains(errorContent.Text, tc.expectedErrMsg) {
					return // This is expected for parameter validation errors
				}
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.IsError)
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedIssueTypes []*github.IssueType
			err = json.Unmarshal([]byte(textContent.Text), &returnedIssueTypes)
			require.NoError(t, err)

			if tc.expectedIssueTypes != nil {
				require.Equal(t, len(tc.expectedIssueTypes), len(returnedIssueTypes))
				for i, expected := range tc.expectedIssueTypes {
					assert.Equal(t, *expected.Name, *returnedIssueTypes[i].Name)
					assert.Equal(t, *expected.Description, *returnedIssueTypes[i].Description)
					assert.Equal(t, *expected.Color, *returnedIssueTypes[i].Color)
					assert.Equal(t, *expected.ID, *returnedIssueTypes[i].ID)
				}
			}
		})
	}
}
