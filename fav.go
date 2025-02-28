package fav

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/net/proxy"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Common error definitions using sentinel pattern
var (
	ErrInvalidURL         = errors.New("invalid URL")
	ErrDomainExtraction   = errors.New("failed to extract domain")
	ErrFaviconNotFound    = errors.New("no favicon found")
	ErrRequestFailed      = errors.New("request failed")
	ErrResponseReadFailed = errors.New("failed to read response")
	ErrSaveFailed         = errors.New("failed to save favicon")
	ErrDirectoryCreation  = errors.New("failed to create directory")
	ErrInvalidProxy       = errors.New("invalid proxy configuration")
)

// ProxyType represents the type of proxy
type ProxyType int

const (
	// NoProxy indicates no proxy should be used
	NoProxy ProxyType = iota
	// HTTPProxy for HTTP proxies
	HTTPProxy
	// Socks5Proxy for SOCKS5 proxies
	Socks5Proxy
)

// ProxyConfig stores proxy configuration
type ProxyConfig struct {
	// Type of proxy to use
	Type ProxyType
	// URL of the proxy server (e.g., "http://proxy.example.com:8080" or "socks5://proxy.example.com:1080")
	URL string
	// Username for proxy authentication (optional)
	Username string
	// Password for proxy authentication (optional)
	Password string
}

// Options defines configuration options for favicon fetching
type Options struct {
	// Timeout for requests, default 3 seconds
	Timeout time.Duration
	// PreferFallback determines if third-party services should be tried first
	PreferFallback bool
	// MaxConcurrentRequests limits concurrent fetching operations
	MaxConcurrentRequests int
	// UserAgent sent with requests
	UserAgent string
	// Proxy configuration
	Proxy ProxyConfig
	// DisableFallbackServices skips external services like Google when true
	DisableFallbackServices bool
}

// DefaultOptions returns the default configuration
func DefaultOptions() Options {
	return Options{
		Timeout:                 3 * time.Second,
		PreferFallback:          false,
		MaxConcurrentRequests:   3,
		UserAgent:               "FaviconFetcher/1.0",
		Proxy:                   ProxyConfig{Type: NoProxy},
		DisableFallbackServices: false,
	}
}

// Fetcher is the main favicon retrieval service
type Fetcher struct {
	client  *http.Client
	options Options
}

// Result represents a favicon fetch result
type Result struct {
	URL       string
	Data      []byte
	Error     error
	Source    string
	Timestamp time.Time
	// ContentType stores the content type of the favicon
	ContentType string
}

// New creates a new favicon fetcher with the provided options
func New(opts ...func(*Options)) (*Fetcher, error) {
	options := DefaultOptions()

	for _, opt := range opts {
		opt(&options)
	}

	// Create HTTP client with configured transport
	client, err := createHTTPClient(options)
	if err != nil {
		return nil, err
	}

	return &Fetcher{
		client:  client,
		options: options,
	}, nil
}

// createHTTPClient creates an HTTP client with proxy support if configured
func createHTTPClient(options Options) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Configure proxy if specified
	if options.Proxy.Type != NoProxy {
		switch options.Proxy.Type {
		case HTTPProxy:
			proxyURL, err := url.Parse(options.Proxy.URL)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidProxy, err)
			}

			if options.Proxy.Username != "" && options.Proxy.Password != "" {
				proxyURL.User = url.UserPassword(options.Proxy.Username, options.Proxy.Password)
			} else if options.Proxy.Username != "" {
				proxyURL.User = url.User(options.Proxy.Username)
			}

			transport.Proxy = http.ProxyURL(proxyURL)

		case Socks5Proxy:
			// Parse the SOCKS5 proxy URL
			proxyURL, err := url.Parse(options.Proxy.URL)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidProxy, err)
			}

			// Get the host:port
			proxyHost := proxyURL.Host

			// Create SOCKS5 dialer
			var auth *proxy.Auth
			if options.Proxy.Username != "" {
				auth = &proxy.Auth{
					User:     options.Proxy.Username,
					Password: options.Proxy.Password,
				}
			}

			dialer, err := proxy.SOCKS5("tcp", proxyHost, auth, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidProxy, err)
			}

			// Set up the transport to use the SOCKS5 dialer
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	}

	return &http.Client{
		Timeout:   options.Timeout,
		Transport: transport,
	}, nil
}

// WithTimeout sets the request timeout duration
func WithTimeout(timeout time.Duration) func(*Options) {
	return func(o *Options) {
		o.Timeout = timeout
	}
}

// WithPreferFallback configures whether to prefer third-party services
func WithPreferFallback(prefer bool) func(*Options) {
	return func(o *Options) {
		o.PreferFallback = prefer
	}
}

// WithMaxConcurrentRequests sets the maximum concurrent requests
func WithMaxConcurrentRequests(max int) func(*Options) {
	return func(o *Options) {
		if max > 0 {
			o.MaxConcurrentRequests = max
		}
	}
}

// WithUserAgent sets the User-Agent header for requests
func WithUserAgent(userAgent string) func(*Options) {
	return func(o *Options) {
		o.UserAgent = userAgent
	}
}

// WithHTTPProxy configures an HTTP proxy
func WithHTTPProxy(proxyURL, username, password string) func(*Options) {
	return func(o *Options) {
		o.Proxy = ProxyConfig{
			Type:     HTTPProxy,
			URL:      proxyURL,
			Username: username,
			Password: password,
		}
	}
}

// WithSocks5Proxy configures a SOCKS5 proxy
func WithSocks5Proxy(proxyURL, username, password string) func(*Options) {
	return func(o *Options) {
		o.Proxy = ProxyConfig{
			Type:     Socks5Proxy,
			URL:      proxyURL,
			Username: username,
			Password: password,
		}
	}
}

// WithDisableFallbackServices disables external services like Google
func WithDisableFallbackServices(disable bool) func(*Options) {
	return func(o *Options) {
		o.DisableFallbackServices = disable
	}
}

// extractDomain extracts the domain from a URL string
func extractDomain(rawURL string) (string, error) {
	// Ensure URL has protocol prefix
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDomainExtraction, err)
	}

	return parsedURL.Hostname(), nil
}

// Fetch retrieves the first available favicon for the specified URL
func (f *Fetcher) Fetch(ctx context.Context, targetURL string) (*Result, error) {
	domain, err := extractDomain(targetURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	sources := f.getSources(domain)

	// Create a result channel and error counter
	results := make(chan *Result, 1)
	var wg sync.WaitGroup

	// Create semaphore to limit concurrent requests
	sem := make(chan struct{}, f.options.MaxConcurrentRequests)

	// Launch goroutines to try each source
	for _, source := range sources {
		wg.Add(1)

		go func(src string) {
			defer wg.Done()

			// Acquire semaphore to limit concurrency
			select {
			case sem <- struct{}{}:
				// Successfully acquired semaphore
				defer func() { <-sem }()
			case <-ctx.Done():
				// Context cancelled
				return
			}

			// Create request
			req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
			if err != nil {
				return
			}

			req.Header.Set("User-Agent", f.options.UserAgent)

			// Send request
			resp, err := f.client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			// Check status code
			if resp.StatusCode != http.StatusOK {
				return
			}

			// Read response body
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}

			// Send result
			select {
			case results <- &Result{
				URL:         src,
				Data:        data,
				Error:       nil,
				Source:      src,
				Timestamp:   time.Now(),
				ContentType: resp.Header.Get("Content-Type"),
			}:
				// Result sent
			default:
				// Channel is full (result already found)
			}
		}(source)
	}

	// Wait for all requests to complete or find a result
	go func() {
		wg.Wait()
		close(results)
	}()

	// Wait for result or context cancellation
	select {
	case result := <-results:
		if result != nil {
			return result, nil
		}
		return nil, ErrFaviconNotFound
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getSources returns all possible favicon sources
func (f *Fetcher) getSources(domain string) []string {
	standardSources := []string{
		fmt.Sprintf("https://%s/favicon.ico", domain),
		fmt.Sprintf("https://%s/logo.svg", domain),
		fmt.Sprintf("https://%s/logo.png", domain),
		fmt.Sprintf("https://%s/apple-touch-icon.png", domain),
		fmt.Sprintf("https://%s/apple-touch-icon-precomposed.png", domain),
		fmt.Sprintf("https://%s/static/img/favicon.ico", domain),
		fmt.Sprintf("https://%s/static/img/favicon.png", domain),
		fmt.Sprintf("https://%s/img/favicon.png", domain),
		fmt.Sprintf("https://%s/img/favicon.ico", domain),
		fmt.Sprintf("https://%s/static/img/logo.svg", domain),
		fmt.Sprintf("https://%s/apple-touch-icon-precomposed.png", domain),
	}

	fallbackServices := []string{}

	// Only add fallback services if not disabled
	if !f.options.DisableFallbackServices {
		fallbackServices = []string{
			fmt.Sprintf("https://www.google.com/s2/favicons?domain=https://%s&sz=64", domain),
			fmt.Sprintf("https://www.google.com/s2/favicons?domain=http://%s&sz=64", domain),
			fmt.Sprintf("https://icons.duckduckgo.com/ip3/%s.ico", domain),
		}
	}

	if f.options.PreferFallback {
		return append(fallbackServices, standardSources...)
	}

	return append(standardSources, fallbackServices...)
}

// FetchAll retrieves all available favicons
func (f *Fetcher) FetchAll(ctx context.Context, targetURL string) ([]*Result, error) {
	domain, err := extractDomain(targetURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	sources := f.getSources(domain)

	// Create a result channel
	resultsChan := make(chan *Result, len(sources))
	var wg sync.WaitGroup

	// Create semaphore to limit concurrent requests
	sem := make(chan struct{}, f.options.MaxConcurrentRequests)

	// Launch goroutines to try each source
	for _, source := range sources {
		wg.Add(1)

		go func(src string) {
			defer wg.Done()

			// Acquire semaphore to limit concurrency
			select {
			case sem <- struct{}{}:
				// Successfully acquired semaphore
				defer func() { <-sem }()
			case <-ctx.Done():
				// Context cancelled
				return
			}

			// Create request
			req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
			if err != nil {
				resultsChan <- &Result{URL: src, Error: err}
				return
			}

			req.Header.Set("User-Agent", f.options.UserAgent)

			// Send request
			resp, err := f.client.Do(req)
			if err != nil {
				resultsChan <- &Result{URL: src, Error: fmt.Errorf("%w: %v", ErrRequestFailed, err)}
				return
			}
			defer resp.Body.Close()

			// Check status code
			if resp.StatusCode != http.StatusOK {
				resultsChan <- &Result{
					URL:   src,
					Error: fmt.Errorf("%w: HTTP %d", ErrRequestFailed, resp.StatusCode),
				}
				return
			}

			// Read response body
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				resultsChan <- &Result{URL: src, Error: fmt.Errorf("%w: %v", ErrResponseReadFailed, err)}
				return
			}

			// Send result
			resultsChan <- &Result{
				URL:         src,
				Data:        data,
				Error:       nil,
				Source:      src,
				Timestamp:   time.Now(),
				ContentType: resp.Header.Get("Content-Type"),
			}
		}(source)
	}

	// Wait for all requests to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect all results
	var results []*Result
	for result := range resultsChan {
		if result != nil && result.Error == nil {
			results = append(results, result)
		}
	}

	if len(results) == 0 {
		return nil, ErrFaviconNotFound
	}

	return results, nil
}

// SaveToFile saves the favicon to a file
func (r *Result) SaveToFile(filePath string) error {
	if r.Error != nil {
		return r.Error
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("%w: %v", ErrDirectoryCreation, err)
		}
	}

	if err := os.WriteFile(filePath, r.Data, 0644); err != nil {
		return fmt.Errorf("%w: %v", ErrSaveFailed, err)
	}

	return nil
}

// SaveToDirectory saves the favicon to a directory with an appropriate filename
func (r *Result) SaveToDirectory(dirPath string) (string, error) {
	if r.Error != nil {
		return "", r.Error
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("%w: %v", ErrDirectoryCreation, err)
	}

	// Generate filename based on source URL
	urlObj, err := url.Parse(r.Source)
	if err != nil {
		return "", fmt.Errorf("%w: invalid source URL", ErrSaveFailed)
	}

	// Extract filename from path or generate one
	filename := filepath.Base(urlObj.Path)
	if filename == "" || filename == "." || filename == "/" {
		// Generate filename based on content type
		ext := ".ico"
		switch {
		case strings.Contains(r.ContentType, "svg"):
			ext = ".svg"
		case strings.Contains(r.ContentType, "png"):
			ext = ".png"
		case strings.Contains(r.ContentType, "jpg"), strings.Contains(r.ContentType, "jpeg"):
			ext = ".jpg"
		}

		host := urlObj.Hostname()
		timestamp := r.Timestamp.Format("20060102-150405")
		filename = fmt.Sprintf("favicon_%s_%s%s", host, timestamp, ext)
	}

	// Ensure filename is valid
	filename = strings.ReplaceAll(filename, ":", "_")
	filename = strings.ReplaceAll(filename, "?", "_")
	filename = strings.ReplaceAll(filename, "&", "_")
	filename = strings.ReplaceAll(filename, "=", "_")

	fullPath := filepath.Join(dirPath, filename)

	// Save the file
	if err := os.WriteFile(fullPath, r.Data, 0644); err != nil {
		return "", fmt.Errorf("%w: %v", ErrSaveFailed, err)
	}

	return fullPath, nil
}

// SaveAllToDirectory saves all favicon results to a directory
func SaveAllToDirectory(results []*Result, dirPath string) ([]string, error) {
	if len(results) == 0 {
		return nil, ErrFaviconNotFound
	}

	var savedPaths []string
	var errors []error

	for _, result := range results {
		if result.Error != nil {
			errors = append(errors, result.Error)
			continue
		}

		path, err := result.SaveToDirectory(dirPath)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		savedPaths = append(savedPaths, path)
	}

	if len(savedPaths) == 0 {
		if len(errors) > 0 {
			return nil, fmt.Errorf("%w: %v", ErrSaveFailed, errors[0])
		}
		return nil, ErrFaviconNotFound
	}

	return savedPaths, nil
}
