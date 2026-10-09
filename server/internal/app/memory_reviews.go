package app

import (
	"context"
	"sort"
	"time"
)

func (r *MemoryRepository) UpsertReview(_ context.Context, review Review) (Review, []string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if review.ImageKeys == nil {
		review.ImageKeys = []string{}
	}
	key := review.UserID + "\x00" + review.TargetType + "\x00" + review.TargetID
	now := time.Now().UnixMilli()
	removed := []string{}
	if id, ok := r.reviewKeys[key]; ok {
		old := r.reviews[id]
		removed = removedReviewImageKeys(old.ImageKeys, review.ImageKeys)
		review.ID = id
		review.CreatedAt = old.CreatedAt
	} else {
		review.ID = r.nextReview
		r.nextReview++
		review.CreatedAt = now
		r.reviewKeys[key] = review.ID
	}
	review.UpdatedAt = now
	review.Author = r.socialProfileLocked(review.UserID, review.UserID)
	review.ImageKeys = append([]string(nil), review.ImageKeys...)
	r.reviews[review.ID] = review
	return review, removed, nil
}
func (r *MemoryRepository) ListReviews(_ context.Context, viewer, targetType, targetID string, before int64, limit int) ([]Review, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Review{}
	for _, review := range r.reviews {
		if review.TargetType == targetType && review.TargetID == targetID && (before == 0 || review.ID < before) && !memoryUsersBlocked(r.blocks, viewer, review.UserID) {
			review.Author = r.socialProfileLocked(viewer, review.UserID)
			review.ImageKeys = append([]string(nil), review.ImageKeys...)
			out = append(out, review)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *MemoryRepository) DeleteReview(_ context.Context, user string, id int64) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	review, ok := r.reviews[id]
	if !ok || review.UserID != user {
		return nil, ErrNotFound
	}
	delete(r.reviewKeys, review.UserID+"\x00"+review.TargetType+"\x00"+review.TargetID)
	delete(r.reviews, id)
	delete(r.reviewComments, id)
	return append([]string(nil), review.ImageKeys...), nil
}
func (r *MemoryRepository) AddReviewComment(_ context.Context, comment ReviewComment) (ReviewComment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	review, ok := r.reviews[comment.ReviewID]
	if !ok || memoryUsersBlocked(r.blocks, comment.UserID, review.UserID) {
		return ReviewComment{}, ErrNotFound
	}
	comment.ID = r.nextReviewComment
	r.nextReviewComment++
	comment.CreatedAt = time.Now().UnixMilli()
	comment.Author = r.socialProfileLocked(comment.UserID, comment.UserID)
	r.reviewComments[comment.ReviewID] = append(r.reviewComments[comment.ReviewID], comment)
	return comment, nil
}
func (r *MemoryRepository) ListReviewComments(_ context.Context, viewer string, reviewID, before int64, limit int) ([]ReviewComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.reviews[reviewID]; !ok {
		return nil, ErrNotFound
	}
	items := r.reviewComments[reviewID]
	out := []ReviewComment{}
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		item := items[i]
		if (before == 0 || item.ID < before) && !memoryUsersBlocked(r.blocks, viewer, item.UserID) {
			item.Author = r.socialProfileLocked(viewer, item.UserID)
			out = append(out, item)
		}
	}
	return out, nil
}

func removedReviewImageKeys(previous, next []string) []string {
	kept := make(map[string]bool, len(next))
	for _, key := range next {
		kept[key] = true
	}
	removed := []string{}
	for _, key := range previous {
		if !kept[key] {
			removed = append(removed, key)
		}
	}
	return removed
}
