package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestShortDramaHeadCoalescesAndRefreshesWithoutBlankWait(t *testing.T) {
	var hits atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 2 {
			close(entered)
			<-release
		}
		fmt.Fprintf(w, `{"value":"%d","data":[],"hasMore":false}`, n)
	}))
	defer upstream.Close()
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	g, err := NewGuoguoService(upstream.URL, strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	q := shortDramaQuery{Section: "short_play", Page: 1}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, e := g.feed(context.Background(), q)
			if e != nil || data["value"] != "1" {
				t.Error(data, e)
			}
		}()
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatal("duplicate head requests", hits.Load())
	}
	g.feedMu.Lock()
	g.feeds[q.Section].fetched = time.Now().Add(-3 * time.Minute)
	g.feedMu.Unlock()
	data, err := g.feed(context.Background(), q)
	if err != nil || data["value"] != "1" {
		t.Fatal("stale head was not returned", data, err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	data, err = g.feed(context.Background(), q)
	if err != nil || data["value"] != "1" {
		t.Fatal("refresh blocked cached display")
	}
	g.feedMu.Lock()
	done := g.feedCalls[q.Section].done
	g.feedMu.Unlock()
	unblock()
	<-done
	data, err = g.feed(context.Background(), q)
	if err != nil || data["value"] != "2" || hits.Load() != 2 {
		t.Fatal("fresh head not published", data, err, hits.Load())
	}
	q.Refresh = true
	data, err = g.feed(context.Background(), q)
	if err != nil || data["value"] != "3" {
		t.Fatal("explicit refresh reused old head", data, err)
	}
}

func TestShortDramaPaginationAndSearchDoNotReuseCategoryHead(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, `{"data":[]}`) }))
	defer upstream.Close()
	g, _ := NewGuoguoService(upstream.URL, strings.Repeat("x", 32))
	for _, q := range []shortDramaQuery{{Section: "short_play", Page: 1}, {Section: "short_play", Page: 1}, {Section: "short_play", Page: 2, Offset: 18, Session: "next"}, {Section: "short_play", Page: 2, Offset: 18, Session: "next"}, {Section: "search", Page: 1, Query: "title"}, {Section: "search", Page: 1, Query: "title"}} {
		if _, err := g.feed(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 5 {
		t.Fatal("pagination/search must retain their independent requests", hits.Load())
	}
}
