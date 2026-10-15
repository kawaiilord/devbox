package app

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const shortDramaHeadFresh = 2 * time.Minute
const shortDramaHeadStale = 30 * time.Minute

type shortDramaHead struct {
	data       map[string]any
	fetched    time.Time
	retryAfter time.Time
}

type shortDramaHeadCall struct {
	done chan struct{}
	data map[string]any
	err  error
}

func (g *GuoguoService) fetchFeed(ctx context.Context, q shortDramaQuery) (map[string]any, error) {
	switch q.Section {
	case "search":
		return g.json(ctx, "GET", "/bridge/search", url.Values{"q": {strings.TrimSpace(q.Query)}}, nil)
	case "hot", "real", "comic", "ai":
		return g.json(ctx, "GET", "/bridge/rankings", url.Values{"board": {"hongguo-" + q.Section}, "page": {strconv.Itoa(q.Page)}}, nil)
	default:
		return g.json(ctx, "POST", "/bridge/recommendations", nil, map[string]any{"genre": q.Section, "offset": q.Offset, "sessionId": q.Session, "seen": q.Seen})
	}
}

// Only the seven public category heads are shared. Search and continuation
// cursors always retain the caller's query; authentication runs before this.
func (g *GuoguoService) feed(ctx context.Context, q shortDramaQuery) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.Section == "search" || q.Page != 1 || q.Offset != 0 || q.Session != "" || len(q.Seen) != 0 {
		return g.fetchFeed(ctx, q)
	}
	key, now := q.Section, time.Now()
	g.feedMu.Lock()
	if g.feeds == nil {
		g.feeds = make(map[string]*shortDramaHead)
		g.feedCalls = make(map[string]*shortDramaHeadCall)
	}
	entry := g.feeds[key]
	usable := entry != nil && now.Sub(entry.fetched) < shortDramaHeadStale
	if usable && !q.Refresh && (now.Sub(entry.fetched) < shortDramaHeadFresh || now.Before(entry.retryAfter)) {
		g.feedMu.Unlock()
		return entry.data, nil
	}
	call := g.feedCalls[key]
	if call == nil {
		call = &shortDramaHeadCall{done: make(chan struct{})}
		g.feedCalls[key] = call
		go func() {
			fetchCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			data, err := g.fetchFeed(fetchCtx, q)
			g.feedMu.Lock()
			defer g.feedMu.Unlock()
			if err == nil {
				g.feeds[key] = &shortDramaHead{data: data, fetched: time.Now()}
			} else if old := g.feeds[key]; old != nil {
				old.retryAfter = time.Now().Add(30 * time.Second)
			}
			call.data, call.err = data, err
			delete(g.feedCalls, key)
			close(call.done)
		}()
	}
	g.feedMu.Unlock()
	if usable && !q.Refresh {
		return entry.data, nil
	}
	select {
	case <-call.done:
		return call.data, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
