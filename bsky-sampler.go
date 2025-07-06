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

// BlueskyFetcher stores all the state and methods for fetching Bluesky posts
type BlueskyFetcher struct {
	client *xrpc.Client
	ctx    context.Context
	did    string
	handle string
	posts  *MaxHeap
}

// NewBlueskyFetcher creates a new BlueskyFetcher instance
func NewBlueskyFetcher(handle string) (*BlueskyFetcher, error) {
	client := &xrpc.Client{
		Host: "https://public.api.bsky.app",
	}
	ctx := context.Background()

	// Resolve handle to DID
	did, err := getHandleDid(ctx, client, handle)
	if err != nil {
		return nil, fmt.Errorf("error resolving handle to DID: %w", err)
	}

	fetcher := &BlueskyFetcher{
		client: client,
		ctx:    ctx,
		did:    did,
		handle: handle,
		posts:  &MaxHeap{},
	}

	return fetcher, nil
}

// getHandleDid resolves a Bluesky handle to a DiD
func getHandleDid(ctx context.Context, client *xrpc.Client, handle string) (string, error) {
	resolveResp, err := atproto.IdentityResolveHandle(ctx, client, handle)
	return resolveResp.Did, err
}

// getDidPostList fetches the most recent posts from a given Bluesky handle.
// It returns a slice of PostView objects, which contain the post data.
// If limit is higher than 100, we'd need to use the cursor, but I'm keeping it simple for now.
func (bf *BlueskyFetcher) fetchPostList(limit int64) ([]*bsky.FeedDefs_PostView, error) {
	feed, err := bsky.FeedGetAuthorFeed(bf.ctx, bf.client, bf.did, "", "posts_no_replies", false, limit)
	if err != nil {
		return nil, fmt.Errorf("error fetching feed: %w", err)
	}

	var postList []*bsky.FeedDefs_PostView
	for _, post := range feed.Feed {
		if post.Post.Author.Did == bf.did {
			postList = append(postList, post.Post)
		}
	}
	return postList, nil
}

// updatePosts fetches the 100 most recent posts and updates the post list
func (bf *BlueskyFetcher) updatePosts() error {
	postList, err := bf.fetchPostList(100)
	if err != nil {
		return fmt.Errorf("error fetching posts: %w", err)
	}
	fmt.Printf("Number of posts fetched: %d\n", len(postList))
	for _, post := range postList {
		feedPost := post.Record.Val.(*bsky.FeedPost)
		postData := PostData{
			Text:      feedPost.Text,
			Timestamp: feedPost.CreatedAt,
			Uri:       post.Uri,
		}
		bf.posts.Push(postData)
	}
	return nil
}

// checkForNewPosts checks if there are new posts since the last update.
// If there are new posts, it updates the global post list.
func (bf *BlueskyFetcher) checkForNewPosts() error {
	if bf.posts.Len() == 0 {
		return bf.updatePosts()
	}

	postList, err := bf.fetchPostList(1)
	if err != nil {
		return err
	}

	if len(postList) == 0 {
		return nil
	}

	recentPost := postList[0].Record.Val.(*bsky.FeedPost)
	if recentPost.CreatedAt <= bf.posts.Get(0).Timestamp {
		return nil
	}

	// Look at the 100 most recent posts
	return bf.updatePosts()
}

// getRandomPost returns a random post from the heap
func (bf *BlueskyFetcher) getRandomPost() (PostData, error) {
	if bf.posts.Len() == 0 {
		return PostData{}, fmt.Errorf("no posts available")
	}

	randomIndex := rand.Intn(bf.posts.Len())
	return bf.posts.Get(randomIndex), nil
}

// startPeriodicUpdates starts a goroutine that periodically checks for new posts
func (bf *BlueskyFetcher) startPeriodicUpdates(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			<-ticker.C
			fmt.Println("Checking for new posts")
			if err := bf.checkForNewPosts(); err != nil {
				log.Printf("Error checking for new posts: %v", err)
			}
		}
	}()
}

// randomPostHandler returns a random post as JSON over HTTP
func (bf *BlueskyFetcher) randomPostHandler(w http.ResponseWriter, r *http.Request) {
	randomPost, err := bf.getRandomPost()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(randomPost); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}
}

// main function initializes the HTTP server and starts the periodic post update.
func main() {
	handle := "carl.cx"
	fmt.Printf("Fetching data for Bluesky handle: %s\n", handle)

	// Create the fetcher
	fetcher, err := NewBlueskyFetcher(handle)
	if err != nil {
		log.Fatalf("Error creating Bluesky fetcher: %v", err)
	}

	// Initialize the recent post list
	if err := fetcher.updatePosts(); err != nil {
		log.Fatalf("Error initializing posts: %v", err)
	}

	// Set up HTTP handler
	http.HandleFunc("/", fetcher.randomPostHandler)

	// Start periodic updates
	fetcher.startPeriodicUpdates(1 * time.Hour)

	log.Fatal(http.ListenAndServe(":80", nil))
}
