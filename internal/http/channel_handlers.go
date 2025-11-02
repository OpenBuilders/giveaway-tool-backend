package http

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	rcache "github.com/open-builders/giveaway-backend/internal/cache/redis"
	tg "github.com/open-builders/giveaway-backend/internal/service/telegram"
)

// ChannelHandlers exposes channel-related endpoints backed by Telegram client.
type ChannelHandlers struct {
	tg      *tg.Client
	avatars *rcache.ChannelAvatarCache
	photos  *rcache.ChannelPhotoCache
}

func NewChannelHandlers(tgc *tg.Client, avatars *rcache.ChannelAvatarCache, photos *rcache.ChannelPhotoCache) *ChannelHandlers {
	return &ChannelHandlers{tg: tgc, avatars: avatars, photos: photos}
}

func (h *ChannelHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/channels/:username/info", h.getChannelInfo)
	r.Get("/channels/:chat/membership", h.checkMembership)
	r.Get("/channels/:chat/boost", h.checkBoost)
}

// RegisterPublicFiber registers public endpoints that don't require authentication
func (h *ChannelHandlers) RegisterPublicFiber(r fiber.Router) {
	r.Get("/channels/:chat/avatar", h.redirectChannelAvatar)
}

func (h *ChannelHandlers) getChannelInfo(c *fiber.Ctx) error {
	username := c.Params("username")
	info, err := h.tg.GetPublicChannelInfo(c.Context(), username)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(info)
}

func (h *ChannelHandlers) checkMembership(c *fiber.Ctx) error {
	chat := c.Params("chat")
	userID, err := c.QueryInt("user_id", 0), error(nil)
	if userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing user_id"})
	}
	ok, err := h.tg.CheckMembership(c.Context(), int64(userID), chat)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": ok})
}

func (h *ChannelHandlers) checkBoost(c *fiber.Ctx) error {
	chat := c.Params("chat")
	userID := c.QueryInt("user_id", 0)
	if userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing user_id"})
	}
	ok, err := h.tg.CheckBoost(c.Context(), int64(userID), chat)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": ok})
}

// redirectChannelAvatar proxies channel avatar from Telegram file URL to avoid exposing bot token,
// using Redis cache keyed by numeric chat ID and big_file_unique_id for change detection.
func (h *ChannelHandlers) redirectChannelAvatar(c *fiber.Ctx) error {
	chatParam := c.Params("chat")
	// Normalize: if looks like username without '@', add it; allow numeric ids as-is
	if chatParam != "" && chatParam[0] != '@' {
		if _, err := strconv.ParseInt(chatParam, 10, 64); err != nil {
			// not numeric, treat as username
			chatParam = "@" + strings.TrimPrefix(chatParam, "@")
		}
	}

	var (
		chatID          int64
		bigFileID       string
		bigFileUniqueID string
	)

	// Try short-lived cache for chat photo identifiers
	if h.photos != nil {
		if entry, err := h.photos.Get(c.Context(), chatParam); err == nil && entry != nil {
			chatID = entry.ID
			bigFileID = entry.BigFileID
			bigFileUniqueID = entry.BigFileUniqueID
		}
	}
	if chatID == 0 || bigFileID == "" || bigFileUniqueID == "" {
		// Fallback to Telegram getChat
		ch, err := h.tg.GetChatRaw(c.Context(), chatParam)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if ch.Photo == nil || ch.Photo.BigFileID == "" || ch.Photo.BigFileUniqueID == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "avatar not set"})
		}
		chatID = ch.ID
		bigFileID = ch.Photo.BigFileID
		bigFileUniqueID = ch.Photo.BigFileUniqueID
		if h.photos != nil {
			_ = h.photos.Set(c.Context(), chatParam, &rcache.ChannelPhotoEntry{ID: chatID, BigFileID: bigFileID, BigFileUniqueID: bigFileUniqueID})
		}
	}

	var filePath string
	if h.avatars != nil {
		if entry, err := h.avatars.Get(c.Context(), chatID); err == nil && entry != nil {
			if entry.BigFileUniqueID == bigFileUniqueID && entry.FilePath != "" {
				filePath = entry.FilePath
			}
		}
	}

	if filePath == "" {
		// Cache miss or outdated unique_id: resolve via getFile
		fp, err := h.tg.GetFilePath(c.Context(), bigFileID)
		if err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
		}
		filePath = fp
		if h.avatars != nil {
			_ = h.avatars.Set(c.Context(), chatID, &rcache.ChannelAvatarEntry{FilePath: filePath, BigFileUniqueID: bigFileUniqueID})
		}
	}

	// Proxy the image from Telegram to avoid exposing bot token
	fileURL := h.tg.BuildFileURL(filePath)
	req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, fileURL, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create request"})
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "failed to fetch avatar"})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Avatar might have been deleted/changed; invalidate cache and return error
		if h.avatars != nil {
			_ = h.avatars.Invalidate(c.Context(), chatID)
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "avatar not available"})
	}

	// Set appropriate headers
	c.Set("Content-Type", resp.Header.Get("Content-Type"))
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		c.Set("Content-Length", contentLength)
	}
	// Cache control for client-side caching (24 hours)
	c.Set("Cache-Control", "public, max-age=86400")

	// Stream the image directly to the response
	_, err = io.Copy(c.Response().BodyWriter(), resp.Body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to stream avatar"})
	}

	return nil
}
