package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

func main() {
	repos, err := readRepoList("repositories.txt")
	if err != nil {
		panic(err)
	}

	client := initGitHubClient()

	var messages []string

	for _, repo := range repos {
		msg, err := processRepository(client, repo)
		if err != nil {
			fmt.Println("Error processing repository:", err)
			continue
		}
		messages = append(messages, msg)
	}

	for _, msg := range messages {
		fmt.Println(msg)
	}
}

func readRepoList(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var repos []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		repos = append(repos, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return repos, nil
}

func initGitHubClient() *github.Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("PAT")},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

func processRepository(client *github.Client, repo string) (string, error) {
	// Split repo string to get owner and repo name
	parts := strings.Split(repo, "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid repository format")
	}
	owner, repoName := parts[1], parts[2]

	ctx := context.Background()

	// Check if CODEOWNERS file exists
	_, _, resp, err := client.Repositories.GetContents(ctx, owner, repoName, ".github/CODEOWNERS", nil)
	if err == nil {
		// File exists, return message
		return "CODEOWNERS File Exist", nil
	}

	// If the file does not exist, resp will be non-nil and we can check the HTTP status code
	if resp != nil && resp.StatusCode != 404 {
		// An error other than 'not found' occurred, handle appropriately
		return "", fmt.Errorf("error checking CODEOWNERS file: %v", err)
	}

	repoInfo, _, err := client.Repositories.Get(ctx, owner, repoName)
	if err != nil {
		return "", fmt.Errorf("error getting repository info: %v", err)
	}
	baseBranch := repoInfo.GetDefaultBranch()
	baseBranchRef, _, err := client.Git.GetRef(ctx, owner, repoName, "refs/heads/"+baseBranch)
	if err != nil {
		return "", fmt.Errorf("error getting reference for base branch: %v", err)
	}
	baseSHA := baseBranchRef.GetObject().GetSHA()

	// Create a new branch
	newBranch := "create-codeowners-" + baseSHA[:7]
	newBranchRef := "refs/heads/" + newBranch
	ref := &github.Reference{Ref: &newBranchRef, Object: &github.GitObject{SHA: &baseSHA}}
	_, _, err = client.Git.CreateRef(ctx, owner, repoName, ref)
	if err != nil {
		return "", fmt.Errorf("error creating new branch: %v", err)
	}

	// Create a CODEOWNERS file on the new branch
	opt := &github.RepositoryContentFileOptions{
		Message: github.String("Create CODEOWNERS file"),
		Content: []byte("* @org/team"),
		Branch:  &newBranch,
	}
	_, _, err = client.Repositories.CreateFile(ctx, owner, repoName, ".github/CODEOWNERS", opt)
	if err != nil {
		return "", fmt.Errorf("error creating CODEOWNERS file: %v", err)
	}

	// Create a pull request
	prTitle := "Add CODEOWNERS file"
	prBody := "This PR adds a CODEOWNERS file to define the team responsible for this repo."
	newPR := &github.NewPullRequest{
		Title: &prTitle,
		Head:  &newBranch,
		Base:  &baseBranch,
		Body:  &prBody,
	}

	pr, _, err := client.PullRequests.Create(ctx, owner, repoName, newPR)
	if err != nil {
		return "", fmt.Errorf("error creating pull request: %v", err)
	}

	return *pr.HTMLURL, nil
}
