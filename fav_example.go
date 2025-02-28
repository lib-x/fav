package fav

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

func Example() {
	// Example 1: Basic usage without proxy
	basicExample()

	// Example 2: Using HTTP proxy
	httpProxyExample()

	// Example 3: Using SOCKS5 proxy
	socks5ProxyExample()

	// Example 4: Disable fallback services (for regions where they're blocked)
	noFallbackExample()
}

func basicExample() {
	fmt.Println("=== Basic Example ===")

	// Create a favicon fetcher with custom options
	fetcher, err := New(
		WithTimeout(5*time.Second),
		WithMaxConcurrentRequests(5),
		WithUserAgent("MyApp/1.0"),
	)

	if err != nil {
		log.Fatalf("Failed to create fetcher: %v", err)
	}

	// Create context with timeout for cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetch favicon
	result, err := fetcher.Fetch(ctx, "github.com")
	if err != nil {
		if errors.Is(err, ErrFaviconNotFound) {
			log.Println("No favicon found for github.com")
		} else {
			log.Printf("Failed to fetch favicon: %v", err)
		}
		return
	}

	fmt.Printf("Successfully fetched favicon from: %s, size: %d bytes\n",
		result.Source, len(result.Data))

	// Save to directory
	savedPath, err := result.SaveToDirectory("favicons")
	if err != nil {
		log.Printf("Failed to save favicon: %v", err)
		return
	}

	fmt.Printf("Favicon saved to %s\n", savedPath)
}

func httpProxyExample() {
	fmt.Println("\n=== HTTP Proxy Example ===")

	// Create a favicon fetcher with HTTP proxy
	fetcher, err := New(
		WithTimeout(8*time.Second), // Longer timeout for proxy connections
		WithHTTPProxy(
			"http://proxy.example.com:8080", // Replace with your HTTP proxy
			"username",                      // Optional: username for authentication
			"password",                      // Optional: password for authentication
		),
	)

	if err != nil {
		log.Printf("Failed to create fetcher with HTTP proxy: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Try to fetch favicon for a site that might be accessed through proxy
	result, err := fetcher.Fetch(ctx, "google.com")
	if err != nil {
		log.Printf("Failed to fetch favicon through HTTP proxy: %v", err)
		return
	}

	fmt.Printf("Successfully fetched favicon through HTTP proxy from: %s\n", result.Source)

	// Save to directory
	savedPath, err := result.SaveToDirectory("proxy_favicons")
	if err != nil {
		log.Printf("Failed to save favicon: %v", err)
		return
	}

	fmt.Printf("Favicon saved to %s\n", savedPath)
}

func socks5ProxyExample() {
	fmt.Println("\n=== SOCKS5 Proxy Example ===")

	// Create a favicon fetcher with SOCKS5 proxy
	fetcher, err := New(
		WithTimeout(8*time.Second), // Longer timeout for proxy connections
		WithSocks5Proxy(
			"socks5://127.0.0.1:1080", // Replace with your SOCKS5 proxy
			"username",                // Optional: username for authentication
			"password",                // Optional: password for authentication
		),
	)

	if err != nil {
		log.Printf("Failed to create fetcher with SOCKS5 proxy: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Try to fetch favicon for a site that might require proxy access
	result, err := fetcher.Fetch(ctx, "twitter.com")
	if err != nil {
		log.Printf("Failed to fetch favicon through SOCKS5 proxy: %v", err)
		return
	}

	fmt.Printf("Successfully fetched favicon through SOCKS5 proxy from: %s\n", result.Source)

	savedPath, err := result.SaveToDirectory("proxy_favicons")
	if err != nil {
		log.Printf("Failed to save favicon: %v", err)
		return
	}

	fmt.Printf("Favicon saved to %s\n", savedPath)
}

func noFallbackExample() {
	fmt.Println("\n=== No Fallback Services Example ===")

	// Create a fetcher that doesn't use Google or other fallback services
	// Useful in regions where these services might be blocked
	fetcher, err := New(
		WithDisableFallbackServices(true),
		WithSocks5Proxy(
			"socks5://127.0.0.1:1080", // Optional: use proxy if needed
			"",
			"",
		),
	)

	if err != nil {
		log.Printf("Failed to create fetcher: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to fetch favicon directly from the website
	websites := []string{
		"baidu.com",
		"taobao.com",
		"qq.com",
	}

	for _, site := range websites {
		fmt.Printf("Fetching favicon for %s (direct, no fallback)...\n", site)
		result, err := fetcher.Fetch(ctx, site)
		if err != nil {
			fmt.Printf("Failed to fetch favicon for %s: %v\n", site, err)
			continue
		}

		fmt.Printf("Successfully fetched favicon for %s from: %s\n", site, result.Source)

		savedPath, err := result.SaveToDirectory("china_favicons")
		if err != nil {
			fmt.Printf("Failed to save favicon: %v\n", err)
			continue
		}

		fmt.Printf("Saved favicon to %s\n", savedPath)
	}
}
