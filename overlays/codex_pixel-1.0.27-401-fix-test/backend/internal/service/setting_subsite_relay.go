package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

type cachedSubsiteForwardMode struct {
	value     string
	expiresAt int64
}

var subsiteForwardModeCache atomic.Value
var subsiteForwardModeSF singleflight.Group

const subsiteForwardModeCacheTTL = 60 * time.Second
const subsiteForwardModeErrorTTL = 5 * time.Second
const subsiteForwardModeDBTimeout = 5 * time.Second

func NormalizeSubsiteForwardMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "local", "master", "master_local":
		return "local"
	case "direct", "subsite", "subsite_direct":
		return "direct"
	case "forward", "subsite_forward", "":
		return "forward"
	default:
		return "forward"
	}
}

func (s *SettingService) GetSubsiteForwardMode(ctx context.Context, fallback string) string {
	if cached, ok := subsiteForwardModeCache.Load().(*cachedSubsiteForwardMode); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}
	result, _, _ := subsiteForwardModeSF.Do("subsite_forward_mode", func() (any, error) {
		if cached, ok := subsiteForwardModeCache.Load().(*cachedSubsiteForwardMode); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}
		if s == nil || s.settingRepo == nil {
			return NormalizeSubsiteForwardMode(fallback), nil
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), subsiteForwardModeDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeySubsiteForwardMode)
		if err != nil {
			if !errors.Is(err, ErrSettingNotFound) {
				slog.Warn("failed to get subsite forward mode setting", "error", err)
			}
			mode := NormalizeSubsiteForwardMode(fallback)
			subsiteForwardModeCache.Store(&cachedSubsiteForwardMode{
				value:     mode,
				expiresAt: time.Now().Add(subsiteForwardModeErrorTTL).UnixNano(),
			})
			return mode, nil
		}
		mode := NormalizeSubsiteForwardMode(firstNonEmptyString(value, fallback))
		subsiteForwardModeCache.Store(&cachedSubsiteForwardMode{
			value:     mode,
			expiresAt: time.Now().Add(subsiteForwardModeCacheTTL).UnixNano(),
		})
		return mode, nil
	})
	if mode, ok := result.(string); ok {
		return NormalizeSubsiteForwardMode(mode)
	}
	return NormalizeSubsiteForwardMode(fallback)
}

func (s *SettingService) SetSubsiteForwardMode(ctx context.Context, mode string) (string, error) {
	normalized := NormalizeSubsiteForwardMode(mode)
	if s == nil || s.settingRepo == nil {
		return normalized, ErrSettingNotFound
	}
	if err := s.settingRepo.Set(ctx, SettingKeySubsiteForwardMode, normalized); err != nil {
		return "", err
	}
	subsiteForwardModeSF.Forget("subsite_forward_mode")
	subsiteForwardModeCache.Store(&cachedSubsiteForwardMode{
		value:     normalized,
		expiresAt: time.Now().Add(subsiteForwardModeCacheTTL).UnixNano(),
	})
	return normalized, nil
}
