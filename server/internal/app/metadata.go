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
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const defaultTMDBBaseURL = "https://api.themoviedb.org/3/"

var metadataLanguagePattern = regexp.MustCompile(`^[a-z]{2}-[A-Z]{2}$`)
var tmdbImagePathPattern = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,256}$`)
var tmdbTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type MetadataClient struct {
	token   string
	baseURL *url.URL
	client  *http.Client
}

type tmdbSearchResponse struct {
	Results []struct {
		ID            int64   `json:"id"`
		MediaType     string  `json:"media_type"`
		Title         string  `json:"title"`
		Name          string  `json:"name"`
		OriginalTitle string  `json:"original_title"`
		OriginalName  string  `json:"original_name"`
		Overview      string  `json:"overview"`
		ReleaseDate   string  `json:"release_date"`
		FirstAirDate  string  `json:"first_air_date"`
		PosterPath    string  `json:"poster_path"`
		VoteAverage   float64 `json:"vote_average"`
		Adult         bool    `json:"adult"`
	} `json:"results"`
}

func NewMetadataClient(token string, baseURLs ...string) (*MetadataClient, error) {
	token = strings.TrimSpace(token)
	if len(token) < 16 || len(token) > 4096 || !tmdbTokenPattern.MatchString(token) ||
		strings.ContainsFunc(token, unicode.IsControl) {
		return nil, errors.New("invalid TMDB token")
	}
	baseURL := defaultTMDBBaseURL
	if len(baseURLs) > 0 && baseURLs[0] != "" {
		baseURL = baseURLs[0]
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid metadata base URL")
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return &MetadataClient{
		token: token, baseURL: parsed,
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 3 || (len(via) > 0 && sourceAuthority(request.URL) != sourceAuthority(via[0].URL)) {
					return errors.New("unsafe metadata redirect blocked")
				}
				return nil
			},
		},
	}, nil
}

func (c *MetadataClient) Search(
	ctx context.Context,
	query, language string,
) ([]MetadataResult, error) {
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) < 1 ||
		utf8.RuneCountInString(query) > 100 || strings.ContainsFunc(query, unicode.IsControl) {
		return nil, errors.New("metadata query must be 1-100 characters")
	}
	if !metadataLanguagePattern.MatchString(language) {
		language = "zh-CN"
	}
	target := c.baseURL.ResolveReference(&url.URL{Path: "search/multi"})
	values := target.Query()
	values.Set("query", query)
	values.Set("language", language)
	values.Set("include_adult", "false")
	values.Set("page", "1")
	target.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, errors.New("metadata service is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata service returned HTTP %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(payload) > 4<<20 {
		return nil, errors.New("metadata response is too large")
	}
	var decoded tmdbSearchResponse
	if json.Unmarshal(payload, &decoded) != nil {
		return nil, errors.New("invalid metadata response")
	}
	results := make([]MetadataResult, 0, len(decoded.Results))
	for _, item := range decoded.Results {
		if item.Adult || item.ID < 1 || (item.MediaType != "movie" && item.MediaType != "tv") {
			continue
		}
		title, original, releaseDate := item.Title, item.OriginalTitle, item.ReleaseDate
		if item.MediaType == "tv" {
			title, original, releaseDate = item.Name, item.OriginalName, item.FirstAirDate
		}
		title = sanitizeMetadataText(title, 256)
		if title == "" {
			continue
		}
		rating := item.VoteAverage
		if rating < 0 {
			rating = 0
		} else if rating > 10 {
			rating = 10
		}
		result := MetadataResult{
			ID: item.ID, MediaType: item.MediaType, Title: title,
			Original:    sanitizeMetadataText(original, 256),
			Overview:    sanitizeMetadataText(item.Overview, 2000),
			ReleaseDate: sanitizeMetadataText(releaseDate, 32),
			Rating:      rating,
		}
		if tmdbImagePathPattern.MatchString(item.PosterPath) && !strings.Contains(item.PosterPath, "..") {
			result.PosterURL = "https://image.tmdb.org/t/p/w500" + item.PosterPath
		}
		results = append(results, result)
		if len(results) >= 20 {
			break
		}
	}
	return results, nil
}

func sanitizeMetadataText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func (s *Server) searchMetadata(w http.ResponseWriter, r *http.Request) {
	if s.options.Metadata == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("metadata search unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if !s.enforceRateLimit(w, r, "metadata-search", user.ID, 60, time.Minute) {
		return
	}
	results, err := s.options.Metadata.Search(
		r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("language"),
	)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Code: 0, Data: map[string]any{"results": results}, Msg: "ok",
	})
}
