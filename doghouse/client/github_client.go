package client

import (
	"context"

	"github.com/google/go-github/v55/github"

	"github.com/bgpat/reviewdog/doghouse"
	"github.com/bgpat/reviewdog/doghouse/server"
)

// GitHubClient is client which talks to GitHub directly instead of talking to
// doghouse server.
type GitHubClient struct {
	Client *github.Client
}

func (c *GitHubClient) Check(ctx context.Context, req *doghouse.CheckRequest) (*doghouse.CheckResponse, error) {
	return server.NewChecker(req, c.Client).Check(ctx)
}
