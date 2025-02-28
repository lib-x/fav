package fav

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test_Basic(t *testing.T) {
	fetcher, err := New(
		WithTimeout(5*time.Second),
		WithMaxConcurrentRequests(5),
		WithUserAgent("MyApp/1.0"),
	)

	if err != nil {
		t.Fatalf("Failed to create fetcher: %v", err)
	}

	// Create context with timeout for cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetch favicon
	result, err := fetcher.Fetch(ctx, "github.com")
	if err != nil {
		if errors.Is(err, ErrFaviconNotFound) {
			t.Log("No favicon found for github.com")
		} else {
			t.Logf("Failed to fetch favicon: %v", err)
		}
		return
	}

	t.Logf("Successfully fetched favicon from: %s, size: %d bytes\n",
		result.Source, len(result.Data))

	// Save to directory
	savedPath, err := result.SaveToDirectory("testdata")
	if err != nil {
		t.Logf("Failed to save favicon: %v", err)
		return
	}

	t.Logf("Favicon saved to %s\n", savedPath)
}
