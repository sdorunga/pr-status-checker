package main

import (
	"context"
	"fmt"
	gh "github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
	"os"
)

type PRQuery struct {
	Repository struct {
		Description           string
		PullRequestConnection struct {
			Nodes []struct { // PR
				Author struct {
					Login string
				}
				Number                 gh.Int
				Permalink              gh.URI
				Title                  gh.String
				IssueCommentConnection struct {
					Nodes []struct { // IssueComments
						Author struct {
							Login string
						}
						PublishedAt gh.DateTime
						Body        gh.String
					}
					PageInfo struct {
						EndCursor   gh.String
						HasNextPage bool
					}
				} `graphql:"comments(orderBy: {field: UPDATED_AT, direction: DESC}, last: 10)"`
				PullRequestReviewConnection struct {
					Nodes []struct { // PRReviews
						Author struct {
							Login string
						}
						PublishedAt gh.DateTime
						State       gh.PullRequestReviewState
						Body        gh.String
					}
					PageInfo struct {
						EndCursor   gh.String
						HasNextPage bool
					}
				} `graphql:"reviews(author: $reviewauthor, states: $reviewstates, last: 1)"`
				ReviewRequestConnection struct {
					Nodes []struct { // ReviewRequests
						RequestedReviewer struct {
							User struct {
								Login gh.String
								Name  gh.String
							} `graphql:"... on User"`
							Team struct {
								Name gh.String
								// TODO: I could instead check team `members(query: username)` which might give me
								// membership so I don't have to hardcode
							} `graphql:"... on Team"`
						}
						AsCodeOwner gh.Boolean
					}
					PageInfo struct {
						EndCursor   gh.String
						HasNextPage bool
					}
				} `graphql:"reviewRequests(last: 10)"`
			}
			PageInfo struct {
				EndCursor   gh.String
				HasNextPage bool
			}
		} `graphql:"pullRequests(states: $states, last: $last)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

type Interaction struct {
	body string
	at   gh.DateTime
}

func (i Interaction) After(t gh.DateTime) bool {
	return i.at.After(t.Time)
}

func (i Interaction) Before(t gh.DateTime) bool {
	return i.at.Before(t.Time)
}

func main() {
	ctx := context.Background()
	me := os.Getenv("ME")
	ghToken := os.Getenv("TOKEN")
	repoOwner := os.Getenv("OWNER")
	repoName := os.Getenv("REPO")
	token := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: ghToken},
	)
	authedClient := oauth2.NewClient(ctx, token)

	client := gh.NewClient(authedClient)

	var q PRQuery

	err := client.Query(context.Background(), &q, map[string]interface{}{
		"owner":        gh.String(repoOwner),
		"name":         gh.String(repoName),
		"states":       []gh.PullRequestState{gh.PullRequestStateOpen},
		"last":         gh.Int(100),
		"reviewauthor": gh.String(me),
		"reviewstates": []gh.PullRequestReviewState{gh.PullRequestReviewStateCommented,
			gh.PullRequestReviewStateChangesRequested,
			gh.PullRequestReviewStateDismissed,
			gh.PullRequestReviewStatePending},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(q.Repository.Description)

	fmt.Println("My PRs\n")
	for _, pr := range q.Repository.PullRequestConnection.Nodes {
		if pr.Author.Login == me {
			fmt.Printf("#%d, %s \n", int(pr.Number), pr.Permalink)
		}

		//if pr.Number == gh.Int(3870) {
		//	fmt.Printf("HI\n")
		//}
	}

	fmt.Println("\nPRs engaged with\n")
	for _, pr := range q.Repository.PullRequestConnection.Nodes {
		// Skip PRs I own
		if pr.Author.Login == me {
			continue
		}

		prAuthor := pr.Author.Login
		latestAuthorInteraction := Interaction{}

		hasReviewed := false
		hasCommented := false
		hasBeenRequestedToReview := false
		latestMeInteraction := Interaction{}
		for _, review := range pr.PullRequestReviewConnection.Nodes {
			if review.Author.Login == me {
				hasReviewed = true
				if latestMeInteraction.Before(review.PublishedAt) {
					latestMeInteraction.at = review.PublishedAt
					latestMeInteraction.body = string(review.Body)
				}
			}
		}

		for _, comment := range pr.IssueCommentConnection.Nodes {
			if comment.Author.Login == me {
				hasCommented = true
				if latestMeInteraction.Before(comment.PublishedAt) {
					latestMeInteraction.at = comment.PublishedAt
					latestMeInteraction.body = string(comment.Body)
				}
			}

			if comment.Author.Login == prAuthor {
				if latestAuthorInteraction.Before(comment.PublishedAt) {
					latestAuthorInteraction.at = comment.PublishedAt
					latestAuthorInteraction.body = string(comment.Body)
				}
			}
		}

		for _, reviewRequest := range pr.ReviewRequestConnection.Nodes {
			if reviewRequest.RequestedReviewer.Team.Name == "Backend" || reviewRequest.RequestedReviewer.User.Login == gh.String(me) {
				hasBeenRequestedToReview = true
			}
		}

		if hasReviewed || hasCommented || hasBeenRequestedToReview {
			status := "✅"
			latestBody := latestMeInteraction.body
			latestAuthor := me
			if latestMeInteraction.Before(latestAuthorInteraction.at) {
				latestAuthor = prAuthor
				status = "⚠️"
				latestBody = latestAuthorInteraction.body
			}
			fmt.Printf("%s #%d -- %s -- %s \n", status, int(pr.Number), pr.Title, pr.Permalink)
			if len(latestBody) != 0 {
				fmt.Printf("\t%s - %s\n", latestAuthor, shortBody(latestBody))
			}
		}
	}
}

func shortBody(body string) string {
	shortLen := 100
	if len(body) < shortLen {
		return body
	}
	return body[:shortLen] + "..."
}
