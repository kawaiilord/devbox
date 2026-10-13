package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const discoveryPageSize = 24

type DiscoveryItem struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Original string  `json:"original,omitempty"`
	Kind     string  `json:"kind"`
	Rating   float64 `json:"rating"`
	Votes    int     `json:"votes,omitempty"`
	Poster   string  `json:"poster,omitempty"`
	URL      string  `json:"url"`
	Date     string  `json:"date,omitempty"`
	Summary  string  `json:"summary,omitempty"`
	Episodes string  `json:"episodes,omitempty"`
	Weekday  int     `json:"weekday,omitempty"`
	Rank     int     `json:"rank,omitempty"`
	New      bool    `json:"is_new,omitempty"`
}

type DiscoveryFeed struct {
	Section   string          `json:"section"`
	Category  string          `json:"category"`
	Source    string          `json:"source"`
	Page      int             `json:"page"`
	Weekday   int             `json:"weekday,omitempty"`
	Items     []DiscoveryItem `json:"items"`
	HasMore   bool            `json:"has_more"`
	FetchedAt int64           `json:"fetched_at"`
	Stale     bool            `json:"stale"`
}

type discoverySnapshot struct {
	items []DiscoveryItem
	at    time.Time
}

type discoveryPoster struct {
	data []byte
	kind string
	at   time.Time
}

// Only public editorial metadata is cached here. Room and source credentials
// never enter this cache or leave through discovery responses.
type DiscoveryService struct {
	client      *http.Client
	now         func() time.Time
	mu          sync.Mutex
	cache       map[string]discoverySnapshot
	retryAt     map[string]time.Time
	posters     map[string]discoveryPoster
	posterBytes int
	flights     singleflight.Group
	slots       chan struct{}
}

func NewDiscoveryService() *DiscoveryService {
	client := newSourceHTTPClient(false)
	client.Timeout = 12 * time.Second
	return &DiscoveryService{client: client, now: time.Now, cache: map[string]discoverySnapshot{}, retryAt: map[string]time.Time{}, posters: map[string]discoveryPoster{}, slots: make(chan struct{}, 6)}
}

var discoverySeries = map[string]string{"all": "热门", "cn": "国产剧", "us": "美剧", "uk": "英剧", "jp": "日剧", "kr": "韩剧", "hk": "港剧", "anime": "日本动画", "variety": "综艺", "documentary": "纪录片"}

func validateDiscoveryQuery(section, category string, page, weekday int) error {
	if page < 1 || page > 20 || weekday < 0 || weekday > 7 {
		return errors.New("无效的浏览页码或星期")
	}
	switch section {
	case "movies":
		if category != "all" && category != "new" {
			return errors.New("无效的电影分类")
		}
	case "series":
		if _, ok := discoverySeries[category]; !ok {
			return errors.New("无效的剧集分类")
		}
	case "top":
		if category != "movie" && category != "tv" {
			return errors.New("无效的榜单分类")
		}
	case "anime":
		if category != "all" {
			return errors.New("无效的新番分类")
		}
	default:
		return errors.New("无效的发现分类")
	}
	return nil
}

func (s *Server) discovery(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil || !hasRegisteredAccount(user) {
		writeError(w, 401, ErrUnauthorized)
		return
	}
	if !s.enforceRateLimit(w, r, "film-discovery", user.ID, 90, time.Minute) {
		return
	}
	q := r.URL.Query()
	section := q.Get("section")
	if section == "" {
		section = "movies"
	}
	category := q.Get("category")
	if category == "" {
		category = "all"
		if section == "top" {
			category = "movie"
		}
	}
	page, weekday := 1, 0
	if q.Has("page") {
		page, err = strconv.Atoi(q.Get("page"))
		if err != nil {
			page = 0
		}
	}
	if q.Has("weekday") {
		weekday, err = strconv.Atoi(q.Get("weekday"))
		if err != nil {
			weekday = -1
		}
	}
	if err = validateDiscoveryQuery(section, category, page, weekday); err != nil {
		writeError(w, 400, err)
		return
	}
	feed, err := s.options.Discovery.Feed(r.Context(), section, category, page, weekday)
	if err != nil {
		w.Header().Set("Retry-After", "30")
		writeError(w, 503, errors.New("影视资料暂时无法加载，请稍后重试。"))
		return
	}
	for i := range feed.Items {
		feed.Items[i].Poster = s.discoveryPosterURL(feed.Items[i].Poster)
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, 200, apiResponse{Code: 0, Data: feed})
}

func (d *DiscoveryService) Feed(ctx context.Context, section, category string, page, weekday int) (DiscoveryFeed, error) {
	if err := validateDiscoveryQuery(section, category, page, weekday); err != nil {
		return DiscoveryFeed{}, err
	}
	key := fmt.Sprintf("%s/%s/%d", section, category, page)
	if section == "anime" || section == "top" {
		key = section + "/" + category
	}
	snapshot, stale, err := d.cached(ctx, key, func(ctx context.Context) ([]DiscoveryItem, error) {
		if section == "anime" {
			return d.fetchCalendar(ctx)
		}
		return d.fetchDouban(ctx, section, category, page)
	})
	if err != nil {
		return DiscoveryFeed{}, err
	}
	items := append([]DiscoveryItem{}, snapshot.items...)
	feed := DiscoveryFeed{Section: section, Category: category, Source: "豆瓣", Page: page, FetchedAt: snapshot.at.UnixMilli(), Stale: stale}
	if section == "anime" {
		feed.Source = "Bangumi"
		if weekday == 0 {
			weekday = int(d.now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Weekday())
			if weekday == 0 {
				weekday = 7
			}
		}
		feed.Weekday = weekday
		filtered := []DiscoveryItem{}
		for _, item := range items {
			if item.Weekday == weekday {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if section == "top" || section == "anime" {
		start := (page - 1) * discoveryPageSize
		if start >= len(items) {
			items = []DiscoveryItem{}
		} else {
			end := min(start+discoveryPageSize, len(items))
			feed.HasMore = end < len(items)
			items = items[start:end]
		}
	} else {
		feed.HasMore = len(items) == discoveryPageSize && page < 20
	}
	feed.Items = items
	return feed, nil
}

func (d *DiscoveryService) cached(ctx context.Context, key string, fetch func(context.Context) ([]DiscoveryItem, error)) (discoverySnapshot, bool, error) {
	d.mu.Lock()
	old, exists := d.cache[key]
	retry := d.retryAt[key]
	now := d.now()
	d.mu.Unlock()
	if exists && now.Sub(old.at) < 30*time.Minute {
		return old, false, nil
	}
	if now.Before(retry) {
		if exists && now.Sub(old.at) < 24*time.Hour {
			return old, true, nil
		}
		return discoverySnapshot{}, false, errors.New("discovery upstream cooling down")
	}
	ch := d.flights.DoChan("catalog:"+key, func() (any, error) {
		// A caller leaving the page must not cancel a shared upstream request.
		fetchCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		items, err := fetch(fetchCtx)
		d.mu.Lock()
		defer d.mu.Unlock()
		if err != nil {
			d.retryAt[key] = d.now().Add(30 * time.Second)
			return nil, err
		}
		value := discoverySnapshot{items: items, at: d.now()}
		if len(d.cache) >= 64 {
			var oldest string
			var at time.Time
			for k, v := range d.cache {
				if oldest == "" || v.at.Before(at) {
					oldest, at = k, v.at
				}
			}
			delete(d.cache, oldest)
			delete(d.retryAt, oldest)
		}
		d.cache[key] = value
		delete(d.retryAt, key)
		return value, nil
	})
	select {
	case <-ctx.Done():
		return discoverySnapshot{}, false, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			if exists && d.now().Sub(old.at) < 24*time.Hour {
				return old, true, nil
			}
			return discoverySnapshot{}, false, result.Err
		}
		return result.Val.(discoverySnapshot), false, nil
	}
}

func (d *DiscoveryService) read(ctx context.Context, target string, limit int64) ([]byte, error) {
	select {
	case d.slots <- struct{}{}:
		defer func() { <-d.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SameFrame/1.1 (https://sameframe.top)")
	req.Header.Set("Accept", "application/json")
	if req.URL.Hostname() == "movie.douban.com" {
		req.Header.Set("Referer", "https://movie.douban.com/")
	}
	res, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("discovery upstream HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("discovery response too large")
	}
	return data, nil
}

var discoverySubjectID = regexp.MustCompile(`^[0-9]{1,12}$`)
var discoveryDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func (d *DiscoveryService) fetchDouban(ctx context.Context, section, category string, page int) ([]DiscoveryItem, error) {
	kind, tag, count, start := "movie", "热门", discoveryPageSize, (page-1)*discoveryPageSize
	if section == "movies" && category == "new" {
		tag = "最新"
	}
	if section == "series" {
		kind, tag = "tv", discoverySeries[category]
	}
	if section == "top" {
		kind, tag, count, start = category, "豆瓣高分", 100, 0
		if kind == "tv" {
			tag = "热门"
		}
	}
	query := url.Values{"type": {kind}, "tag": {tag}, "sort": {"recommend"}, "page_limit": {strconv.Itoa(count)}, "page_start": {strconv.Itoa(start)}}
	raw, err := d.read(ctx, "https://movie.douban.com/j/search_subjects?"+query.Encode(), 2<<20)
	if err != nil {
		return nil, err
	}
	var data struct {
		Subjects *[]struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Rate     string `json:"rate"`
			Cover    string `json:"cover"`
			Episodes string `json:"episodes_info"`
			New      bool   `json:"is_new"`
		} `json:"subjects"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Subjects == nil {
		return nil, errors.New("invalid discovery catalog")
	}
	items := []DiscoveryItem{}
	seen := map[string]bool{}
	for _, subject := range *data.Subjects {
		title := sanitizeMetadataText(subject.Title, 200)
		if title == "" || !discoverySubjectID.MatchString(subject.ID) || seen[subject.ID] {
			continue
		}
		seen[subject.ID] = true
		rate, _ := strconv.ParseFloat(subject.Rate, 64)
		if !(rate >= 0 && rate <= 10) {
			rate = 0
		}
		if section == "top" && rate < 8 {
			continue
		}
		items = append(items, DiscoveryItem{ID: "douban:" + subject.ID, Title: title, Kind: kind, Rating: rate, Poster: normalizeDiscoveryPoster(subject.Cover), URL: "https://movie.douban.com/subject/" + subject.ID + "/", Episodes: sanitizeMetadataText(subject.Episodes, 100), New: subject.New})
		if len(items) >= count {
			break
		}
	}
	if section == "top" {
		sort.SliceStable(items, func(i, j int) bool { return items[i].Rating > items[j].Rating })
		for i := range items {
			items[i].Rank = i + 1
		}
	}
	return items, nil
}

func (d *DiscoveryService) fetchCalendar(ctx context.Context) ([]DiscoveryItem, error) {
	raw, err := d.read(ctx, "https://api.bgm.tv/calendar", 4<<20)
	if err != nil {
		return nil, err
	}
	var days []struct {
		Weekday struct {
			ID int `json:"id"`
		} `json:"weekday"`
		Items []struct {
			ID      int64  `json:"id"`
			Type    int    `json:"type"`
			Name    string `json:"name"`
			NameCN  string `json:"name_cn"`
			Summary string `json:"summary"`
			Date    string `json:"air_date"`
			NSFW    bool   `json:"nsfw"`
			Rating  struct {
				Score float64 `json:"score"`
				Total int     `json:"total"`
			} `json:"rating"`
			Images struct {
				Large  string `json:"large"`
				Medium string `json:"medium"`
			} `json:"images"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &days) != nil || len(days) == 0 || len(days) > 7 {
		return nil, errors.New("invalid discovery calendar")
	}
	items := []DiscoveryItem{}
	seen := map[int64]bool{}
	for _, day := range days {
		if day.Weekday.ID < 1 || day.Weekday.ID > 7 {
			continue
		}
		for _, entry := range day.Items {
			if entry.NSFW || entry.Type != 2 || entry.ID < 1 || seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			title := entry.NameCN
			if strings.TrimSpace(title) == "" {
				title = entry.Name
			}
			title = sanitizeMetadataText(title, 200)
			if title == "" {
				continue
			}
			poster := normalizeDiscoveryPoster(entry.Images.Large)
			if poster == "" {
				poster = normalizeDiscoveryPoster(entry.Images.Medium)
			}
			rate := entry.Rating.Score
			if !(rate >= 0 && rate <= 10) {
				rate = 0
			}
			date := entry.Date
			if !discoveryDate.MatchString(date) {
				date = ""
			}
			items = append(items, DiscoveryItem{ID: "bangumi:" + strconv.FormatInt(entry.ID, 10), Title: title, Original: sanitizeMetadataText(entry.Name, 200), Kind: "anime", Rating: rate, Votes: max(0, entry.Rating.Total), Poster: poster, URL: fmt.Sprintf("https://bgm.tv/subject/%d", entry.ID), Date: date, Summary: sanitizeMetadataText(entry.Summary, 2000), Weekday: day.Weekday.ID})
			if len(items) >= 500 {
				return items, nil
			}
		}
	}
	return items, nil
}
