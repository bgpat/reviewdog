package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/reviewdog/reviewdog"
	"github.com/reviewdog/reviewdog/filter"
	"github.com/reviewdog/reviewdog/proto/rdf"
	"github.com/reviewdog/reviewdog/service/commentutil"
	"github.com/xanzy/go-gitlab"
)

func TestGitLabMergeRequestDiscussionCommenter_Post_Flush_review_api(t *testing.T) {
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	os.Chdir("../..")

	alreadyCommented1 := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "file.go",
					Range: &rdf.Range{Start: &rdf.Position{
						Line: 1,
					}},
				},
				Message: "already commented",
			},
			InDiffFile: true,
		},
	}
	alreadyCommented2 := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "another/file.go",
					Range: &rdf.Range{Start: &rdf.Position{
						Line: 14,
					}},
				},
				Message: "already commented 2",
			},
			InDiffFile: true,
		},
	}
	newComment1 := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "file.go",
					Range: &rdf.Range{Start: &rdf.Position{
						Line: 14,
					}},
				},
				Message: "new comment",
			},
			InDiffFile: true,
		},
	}
	newComment2 := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "file2.go",
					Range: &rdf.Range{Start: &rdf.Position{
						Line: 15,
					}},
				},
				Message: "new comment 2",
			},
			InDiffFile: true,
		},
	}
	newComment3 := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "new_file.go",
					Range: &rdf.Range{Start: &rdf.Position{
						Line: 14,
					}},
				},
				Message: "new comment 3",
			},
			OldPath:    "old_file.go",
			OldLine:    7,
			InDiffFile: true,
		},
	}
	commentOutsideDiff := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "path.go",
					Range: &rdf.Range{Start: &rdf.Position{
						Line: 14,
					}},
				},
				Message: "comment outside diff",
			},
			InDiffFile: false,
		},
	}
	commentWithoutLnum := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "path.go",
				},
				Message: "comment without lnum",
			},
			InDiffFile: true,
		},
	}

	newCommentWithSuggestion := &reviewdog.Comment{
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Location: &rdf.Location{
					Path: "file3.go",
					Range: &rdf.Range{Start: &rdf.Position{
						Line: 14,
					}},
				},
				Message: "new comment with suggestion",
				Suggestions: []*rdf.Suggestion{
					{
						Text: "line1-fixed\nline2-fixed",
						Range: &rdf.Range{
							Start: &rdf.Position{
								Line: 14,
							},
							End: &rdf.Position{
								Line: 15,
							},
						},
					},
				},
			},
			InDiffFile: true,
		},
	}

	comments := []*reviewdog.Comment{
		alreadyCommented1,
		alreadyCommented2,
		newComment1,
		newComment2,
		newComment3,
		commentOutsideDiff,
		commentWithoutLnum,
		newCommentWithSuggestion,
	}
	var postCalled int32
	const wantPostCalled = 4

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/o/r/merge_requests/14/discussions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			switch r.URL.Query().Get("page") {
			default:
				dls := []*gitlab.Discussion{
					{
						Notes: []*gitlab.Note{
							{
								Body: commentutil.MarkdownComment(alreadyCommented1),
								Position: &gitlab.NotePosition{
									NewPath: alreadyCommented1.Result.Diagnostic.GetLocation().GetPath(),
									NewLine: int(alreadyCommented1.Result.Diagnostic.GetLocation().GetRange().GetStart().GetLine()),
								},
							},
							{
								Body: "unrelated commented",
								Position: &gitlab.NotePosition{
									NewPath: "file.go",
									NewLine: 1,
								},
							},
						},
					},
				}
				w.Header().Add("X-Next-Page", "2")
				if err := json.NewEncoder(w).Encode(dls); err != nil {
					t.Fatal(err)
				}
			case "2":
				dls := []*gitlab.Discussion{
					{
						Notes: []*gitlab.Note{
							{
								Body: commentutil.MarkdownComment(alreadyCommented2),
								Position: &gitlab.NotePosition{
									NewPath: alreadyCommented2.Result.Diagnostic.GetLocation().GetPath(),
									NewLine: int(alreadyCommented2.Result.Diagnostic.GetLocation().GetRange().GetStart().GetLine()),
								},
							},
						},
					},
				}
				if err := json.NewEncoder(w).Encode(dls); err != nil {
					t.Fatal(err)
				}
			}

		case http.MethodPost:
			atomic.AddInt32(&postCalled, 1)
			got := new(gitlab.CreateMergeRequestDiscussionOptions)
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Error(err)
			}
			switch *got.Position.NewPath {
			case "file.go":
				want := &gitlab.CreateMergeRequestDiscussionOptions{
					Body: gitlab.String(commentutil.MarkdownComment(newComment1)),
					Position: &gitlab.PositionOptions{
						BaseSHA:      gitlab.String("xxx"),
						StartSHA:     gitlab.String("xxx"),
						HeadSHA:      gitlab.String("sha"),
						PositionType: gitlab.String("text"),
						NewPath:      gitlab.String("file.go"),
						NewLine:      gitlab.Int(14),
					},
				}
				if diff := cmp.Diff(got, want); diff != "" {
					t.Error(diff)
				}
			case "file2.go":
				want := &gitlab.CreateMergeRequestDiscussionOptions{
					Body: gitlab.String(commentutil.MarkdownComment(newComment2)),
					Position: &gitlab.PositionOptions{
						BaseSHA:      gitlab.String("xxx"),
						StartSHA:     gitlab.String("xxx"),
						HeadSHA:      gitlab.String("sha"),
						PositionType: gitlab.String("text"),
						NewPath:      gitlab.String("file2.go"),
						NewLine:      gitlab.Int(15),
					},
				}
				if diff := cmp.Diff(got, want); diff != "" {
					t.Error(diff)
				}
			case "new_file.go":
				want := &gitlab.CreateMergeRequestDiscussionOptions{
					Body: gitlab.String(commentutil.MarkdownComment(newComment3)),
					Position: &gitlab.PositionOptions{
						BaseSHA:      gitlab.String("xxx"),
						StartSHA:     gitlab.String("xxx"),
						HeadSHA:      gitlab.String("sha"),
						PositionType: gitlab.String("text"),
						NewPath:      gitlab.String("new_file.go"),
						NewLine:      gitlab.Int(14),
						OldPath:      gitlab.String("old_file.go"),
						OldLine:      gitlab.Int(7),
					},
				}
				if diff := cmp.Diff(got, want); diff != "" {
					t.Error(diff)
				}
			case "file3.go":
				suggestions := buildSuggestions(newCommentWithSuggestion)
				bodyExpected := commentutil.MarkdownComment(newCommentWithSuggestion) + "\n\n" + suggestions

				want := &gitlab.CreateMergeRequestDiscussionOptions{
					Body: gitlab.String(bodyExpected),
					Position: &gitlab.PositionOptions{
						BaseSHA:      gitlab.String("xxx"),
						StartSHA:     gitlab.String("xxx"),
						HeadSHA:      gitlab.String("sha"),
						PositionType: gitlab.String("text"),
						NewPath:      gitlab.String("file3.go"),
						NewLine:      gitlab.Int(14),
					},
				}
				if diff := cmp.Diff(got, want); diff != "" {
					t.Error(diff)
				}
			default:
				t.Errorf("got unexpected discussion: %#v", got)
			}
			if err := json.NewEncoder(w).Encode(gitlab.Discussion{}); err != nil {
				t.Fatal(err)
			}
		default:
			t.Errorf("unexpected access: %v %v", r.Method, r.URL)
		}
	})
	mux.HandleFunc("/api/v4/projects/o/r/merge_requests/14", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected access: %v %v", r.Method, r.URL)
		}
		w.Write([]byte(`{"target_project_id": 14, "target_branch": "test-branch"}`))
	})
	mux.HandleFunc("/api/v4/projects/14/repository/branches/test-branch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected access: %v %v", r.Method, r.URL)
		}
		w.Write([]byte(`{"commit": {"id": "xxx"}}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cli, err := gitlab.NewClient("", gitlab.WithBaseURL(ts.URL+"/api/v4"))
	if err != nil {
		t.Fatal(err)
	}

	g, err := NewGitLabMergeRequestDiscussionCommenter(cli, "o", "r", 14, "sha")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range comments {
		if err := g.Post(context.Background(), c); err != nil {
			t.Error(err)
		}
	}
	if err := g.Flush(context.Background()); err != nil {
		t.Errorf("%v", err)
	}
	if postCalled != wantPostCalled {
		t.Errorf("%d discussions posted, but want %d", postCalled, wantPostCalled)
	}
}

func TestBuildSuggestions(t *testing.T) {
	tests := []struct {
		in   *reviewdog.Comment
		want string
	}{
		{
			in: &reviewdog.Comment{
				ToolName: "tool-name",
				Result: &filter.FilteredDiagnostic{
					Diagnostic: &rdf.Diagnostic{
						Message: "no suggestion",
					},
				},
			},
			want: "",
		},
		{
			in: buildTestComment(
				"one suggestion",
				[]*rdf.Suggestion{
					buildTestsSuggestion("line1-fixed\nline2-fixed", 10, 10),
				},
			),
			want: strings.Join([]string{
				"```suggestion:-0+0",
				"line1-fixed",
				"line2-fixed",
				"```",
				"",
			}, "\n"),
		},
		{
			in: buildTestComment(
				"two suggestions",
				[]*rdf.Suggestion{
					buildTestsSuggestion("line1-fixed\nline2-fixed", 10, 11),
					buildTestsSuggestion("line3-fixed\nline4-fixed", 20, 21),
				},
			),
			want: strings.Join([]string{
				"```suggestion:-0+1",
				"line1-fixed",
				"line2-fixed",
				"```",
				"```suggestion:-0+1",
				"line3-fixed",
				"line4-fixed",
				"```",
				"",
			}, "\n"),
		},
		{
			in: buildTestComment(
				"a suggestion that has fenced code block",
				[]*rdf.Suggestion{
					buildTestsSuggestion("```shell\ngit config --global receive.advertisepushoptions true\n```", 10, 12),
				},
			),
			want: strings.Join([]string{
				"````suggestion:-0+2",
				"```shell",
				"git config --global receive.advertisepushoptions true",
				"```",
				"````",
				"",
			}, "\n"),
		},
	}
	for _, tt := range tests {
		suggestion := buildSuggestions(tt.in)
		if suggestion != tt.want {
			t.Errorf("got unexpected suggestion.\ngot:\n%s\nwant:\n%s", suggestion, tt.want)
		}
	}
}

func TestBuildSuggestionsInvalid(t *testing.T) {
	tests := []struct {
		in   *reviewdog.Comment
		want string
	}{
		{
			in: buildTestComment(
				"two suggestions, one without range",
				[]*rdf.Suggestion{
					{
						Text: "line3-fixed\nline4-fixed",
					},
					buildTestsSuggestion("line1-fixed\nline2-fixed", 10, 11),
				},
			),
			want: strings.Join([]string{
				"```suggestion:-0+1",
				"line1-fixed",
				"line2-fixed",
				"```",
				"",
			}, "\n"),
		},
		{
			in: buildTestComment(
				"two suggestions, one without range end",
				[]*rdf.Suggestion{
					{
						Text: "line3-fixed\nline4-fixed",
						Range: &rdf.Range{
							Start: &rdf.Position{
								Line: 20,
							},
						},
					},
					buildTestsSuggestion("line1-fixed\nline2-fixed", 10, 11),
				}),
			want: strings.Join([]string{
				"```suggestion:-0+1",
				"line1-fixed",
				"line2-fixed",
				"```",
				"",
			}, "\n"),
		},
	}
	for _, tt := range tests {
		suggestion := buildSuggestions(tt.in)
		if suggestion != tt.want {
			t.Errorf("got unexpected suggestion.\ngot:\n%s\nwant:\n%s", suggestion, tt.want)
		}
	}
}

func buildTestsSuggestion(text string, start int32, end int32) *rdf.Suggestion {
	return &rdf.Suggestion{
		Text: text,
		Range: &rdf.Range{
			Start: &rdf.Position{
				Line: start,
			},
			End: &rdf.Position{
				Line: end,
			},
		},
	}
}

func buildTestComment(message string, suggestions []*rdf.Suggestion) *reviewdog.Comment {
	return &reviewdog.Comment{
		ToolName: "tool-name",
		Result: &filter.FilteredDiagnostic{
			Diagnostic: &rdf.Diagnostic{
				Message:     message,
				Suggestions: suggestions,
			},
		},
	}
}
