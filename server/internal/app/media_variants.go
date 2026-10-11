package app

import (
	"context"
	"errors"
	"strings"
)

func (m *MediaSourceManager) Variants(ctx context.Context, userID, sourceID, path string) ([]MediaVariant, error) {
	source, secret, err := m.loadSourceSecret(ctx, userID, sourceID)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(source.Type, "remote-") {
		return m.remoteVariants(ctx, source, secret, path)
	}
	if _, ok := platformDefinitions[source.Type]; ok {
		return m.platformVariants(ctx, source, secret, path)
	}
	return []MediaVariant{{ID: "original", Label: "原画", Protocol: "file"}}, nil
}

func (m *MediaSourceManager) PrepareMediaVariant(ctx context.Context, userID, sourceID, path, variant string) (MediaTicket, error) {
	source, secret, err := m.loadSourceSecret(ctx, userID, sourceID)
	if err != nil {
		return MediaTicket{}, err
	}
	if strings.HasPrefix(source.Type, "remote-") {
		return m.prepareRemote(ctx, source, secret, path, variant)
	}
	if _, ok := platformDefinitions[source.Type]; ok {
		return m.preparePlatformTicket(ctx, source, secret, path, variant)
	}
	if variant != "" && variant != "original" {
		return MediaTicket{}, errors.New("此片源不提供所选画质")
	}
	ticket, err := m.PrepareMediaTicket(ctx, userID, sourceID, path)
	ticket.VariantID = variant
	return ticket, err
}
