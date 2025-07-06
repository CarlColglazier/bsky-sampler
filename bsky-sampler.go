package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
)

// Global variables
var allPosts = &MaxHeap{}

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

// impl max heap for PostData
type MaxHeap []PostData

func (h MaxHeap) Len() int { return len(h) }
func (h MaxHeap) Less(i, j int) bool {
	return h[i].Timestamp > h[j].Timestamp
}
func (h MaxHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *MaxHeap) Push(x interface{}) {
	*h = append(*h, x.(PostData))
}
func (h *MaxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Get entry at index
func (h *MaxHeap) Get(index int) PostData {
	if index < 0 || index >= len(*h) {
		panic("Index out of bounds")
	}
	return (*h)[index]
}

func randomPostHandler(w http.ResponseWriter, r *http.Request) {
	if allPosts.Len() == 0 {
		http.Error(w, "No posts available", http.StatusNotFound)
		return
	}

	// Get a random post
	randomIndex := rand.Intn(allPosts.Len())
	randomPost := allPosts.Get(randomIndex)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(randomPost); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}
}

func updatePosts(ctx context.Context, client *xrpc.Client, handle string, limit int64) {
	postList, err := getHandlePostList(ctx, client, handle, 100)
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
		allPosts.Push(postData)
	}
}

func checkForNewPosts(ctx context.Context, client *xrpc.Client, handle string, limit int64) {
	postList, err := getHandlePostList(ctx, client, handle, 1)
	if err != nil {
		log.Printf("Error fetching posts: %v", err)
		return
	}
	recentPost := postList[0].Record.Val.(*bsky.FeedPost)
	if recentPost.CreatedAt <= allPosts.Get(0).Timestamp {
		return
	}
	// Look at the 100 most recent posts
	updatePosts(ctx, client, handle, 100)
}

func main() {
	handle := "carl.cx"
	fmt.Printf("Fetching data for Bluesky handle: %s\n", handle)
	client := &xrpc.Client{
		Host: "https://public.api.bsky.app",
	}
	ctx := context.Background()
	// Initialize the recent post list
	updatePosts(ctx, client, handle, 100)

	http.HandleFunc("/", randomPostHandler)

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			<-ticker.C
			fmt.Println("Checking for new posts")
			checkForNewPosts(ctx, client, handle, 100)
		}
	}()

	log.Fatal(http.ListenAndServe(":80", nil))
}
