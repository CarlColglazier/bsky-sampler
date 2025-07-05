package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
)

// If limit is higher than 100, we'd need to use the cursor, but I'm keeping it simple for now.
func getHandlePostList(ctx context.Context, client *xrpc.Client, handle string, limit int64) ([]*bsky.FeedDefs_PostView, error) {
	// Handle => DiD
	resolveResp, err := atproto.IdentityResolveHandle(ctx, client, handle)
	if err != nil {
		log.Fatalf("Error resolving handle: %v", err)
	}
	did := resolveResp.Did
	// Get recent posts from author feed
	feed, err := bsky.FeedGetAuthorFeed(ctx, client, did, "", "posts_no_replies", false, limit)
	if err != nil {
		log.Fatalf("Oh no! Error fetching feed: %v", err)
	}
	var postList []*bsky.FeedDefs_PostView
	for _, post := range feed.Feed {
		if post.Post.Author.Did == did {
			postList = append(postList, post.Post)
		}
	}
	return postList, nil
}

// Struct with "text", "timestamp", "uri"
type PostData struct {
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	Uri       string `json:"uri"`
}

var allPosts []PostData

func randomPostHandler(w http.ResponseWriter, r *http.Request) {
	if len(allPosts) == 0 {
		http.Error(w, "No posts available", http.StatusNotFound)
		return
	}

	// Get a random post
	randomIndex := rand.Intn(len(allPosts))
	randomPost := allPosts[randomIndex]

	// Set content type to JSON
	w.Header().Set("Content-Type", "application/json")

	// Encode and send the random post
	if err := json.NewEncoder(w).Encode(randomPost); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}
}

func main() {
	handle := "carl.cx"
	fmt.Printf("Fetching data for Bluesky handle: %s\n", handle)
	client := &xrpc.Client{
		Host: "https://public.api.bsky.app",
	}
	ctx := context.Background()
	postList, err := getHandlePostList(ctx, client, handle, 50)
	if err != nil {
		log.Fatalf("Error fetching posts: %v", err)
	}
	fmt.Printf("Number of posts fetched: %d\n", len(postList))
	for _, post := range postList {
		feedPost := post.Record.Val.(*bsky.FeedPost)
		postData := PostData{
			Text:      feedPost.Text,
			Timestamp: feedPost.CreatedAt,
			Uri:       post.Uri,
		}
		allPosts = append(allPosts, postData)
	}

	http.HandleFunc("/", randomPostHandler)

	fmt.Println("Server starting on :7890")
	fmt.Println("Visit http://localhost:7890/ to get a random post")

	log.Fatal(http.ListenAndServe(":7890", nil))
}
