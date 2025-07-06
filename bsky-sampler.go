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

// getHandleDid resolves a Bluesky handle to a DiD
func getHandleDid(ctx context.Context, client *xrpc.Client, handle string) (string, error) {
	resolveResp, err := atproto.IdentityResolveHandle(ctx, client, handle)
	return resolveResp.Did, err
}

// getDidPostList fetches the most recent posts from a given Bluesky handle.
// It returns a slice of PostView objects, which contain the post data.
// If limit is higher than 100, we'd need to use the cursor, but I'm keeping it simple for now.
func getDidPostList(ctx context.Context, client *xrpc.Client, did string, limit int64) ([]*bsky.FeedDefs_PostView, error) {
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

// randomPostHandler returns a random post as JSON over HTTP
// If no posts are available, it returns a 404 error.
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

// updatePosts fetches the 100 most recent posts from a given Bluesky handle
// and updates the global post list.
func updatePosts(ctx context.Context, client *xrpc.Client, did string) {
	postList, err := getDidPostList(ctx, client, did, 100)
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

// checkForNewPosts checks if there are new posts since the last update.
// If there are new posts, it updates the global post list.
func checkForNewPosts(ctx context.Context, client *xrpc.Client, did string) {
	postList, err := getDidPostList(ctx, client, did, 1)
	if err != nil {
		log.Printf("Error fetching posts: %v", err)
		return
	}
	recentPost := postList[0].Record.Val.(*bsky.FeedPost)
	if recentPost.CreatedAt <= allPosts.Get(0).Timestamp {
		return
	}
	// Look at the 100 most recent posts
	updatePosts(ctx, client, did)
}

// main function initializes the HTTP server and starts the periodic post update.
func main() {
	handle := "carl.cx"
	fmt.Printf("Fetching data for Bluesky handle: %s\n", handle)
	client := &xrpc.Client{
		Host: "https://public.api.bsky.app",
	}
	ctx := context.Background()
	// get DiD for handle
	myDid, err := getHandleDid(ctx, client, handle)
	if err != nil {
		log.Fatalf("Error getting handle DiD: %v", err)
	}
	// Initialize the recent post list
	updatePosts(ctx, client, myDid)

	http.HandleFunc("/", randomPostHandler)

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			<-ticker.C
			fmt.Println("Checking for new posts")
			checkForNewPosts(ctx, client, myDid)
		}
	}()

	log.Fatal(http.ListenAndServe(":80", nil))
}
