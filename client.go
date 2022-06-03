package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

type Stop struct {
	error
}

type C struct {
	client *http.Client
	rl     *rate.Limiter
}

func NewC(ctx context.Context) (*C, error) {
	return &C{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
		rl: rate.NewLimiter(rate.Every(time.Second), 3),
	}, nil
}

func NewCWithProxy(proxy string) (*C, error) {
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return nil, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &C{
		client: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}, nil
}

func NewCWithLimiter(every time.Duration, times int) *C {
	return &C{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
		rl: rate.NewLimiter(rate.Every(every), times),
	}
}

func (s *C) SetRateLimiter(every time.Duration, times int) *C {
	s.rl = rate.NewLimiter(rate.Every(every), times)

	return s
}

func (s *C) Do(req *http.Request) (*http.Response, error) {
	if s.rl != nil {
		err := s.rl.Wait(context.Background())
		if err != nil {
			return nil, err
		}
	}

	return s.client.Do(req)
}

func (s *C) Get(url string) (*http.Response, error) {
	if s.rl != nil {
		err := s.rl.Wait(context.Background())
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return s.Do(req)
}

func Retry(attempts int, sleep time.Duration, f func() (*http.Response, error)) (*http.Response, error) {
	resp, err := f()
	if err != nil {
		if s, ok := err.(*Stop); ok {
			// Return the original error for later checking
			return nil, s.error
		}

		if attempts--; attempts > 0 {
			// Add some randomness to prevent creating a Thundering Herd
			jitter := time.Duration(rand.Int63n(int64(sleep)))
			sleep = sleep + jitter/2

			time.Sleep(sleep)

			return Retry(attempts, 2*sleep, f)
		}
		return nil, err
	}

	return resp, nil
}

func RetryRequest(client *C, request *http.Request, attempts int, sleep time.Duration) (*http.Response, error) {
	return Retry(attempts, sleep, func() (*http.Response, error) {
		resp, err := client.Do(request)
		if err != nil {
			return nil, err
		}

		s := resp.StatusCode
		switch {
		case s >= 500:
			// Retry
			return nil, fmt.Errorf("server error: %v", s)
		case s >= 400:
			// Don't retry, it was client's fault
			return nil, Stop{fmt.Errorf("client error: %v", s)}
		default:
			// Happy
			return resp, nil
		}
	})
}
