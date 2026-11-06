package handle

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/service"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type RouterOptions struct {
	APIKey         string
	MaxUploadBytes int64
}

type Handler struct {
	service *service.Service
}

func NewRouter(service *service.Service, options RouterOptions) *gin.Engine {
	handler := &Handler{service: service}

	router := gin.New()
	router.Use(gin.Recovery(), corsMiddleware(), requestCostMiddleware())
	router.GET("/health", handler.health)
	router.GET("/s/:code", handler.redirect)

	api := router.Group("/api", apiKeyMiddleware(options.APIKey))
	api.POST("/links", handler.createLink)
	api.POST("/assets", uploadLimitMiddleware(options.MaxUploadBytes), handler.createAssetLink)
	api.GET("/links/:code", handler.getLinkInfo)
	api.GET("/links", handler.getLinksInfo)
	api.DELETE("/links/:code", handler.deleteLinkInfo)
	return router
}

func apiKeyMiddleware(apiKey string) gin.HandlerFunc {
	configured := strings.TrimSpace(apiKey) != ""
	expected := sha256.Sum256([]byte(apiKey))
	return func(c *gin.Context) {
		headers := c.Request.Header.Values("Authorization")
		var token string
		valid := false
		if len(headers) == 1 {
			scheme, value, found := strings.Cut(headers[0], " ")
			token = value
			valid = found && strings.EqualFold(scheme, "Bearer") && token != "" && !strings.ContainsAny(token, " \t\r\n")
		}
		actual := sha256.Sum256([]byte(token))
		// Compare fixed-length digests so key length does not short-circuit the comparison.
		matches := subtle.ConstantTimeCompare(actual[:], expected[:])
		if !configured || !valid || matches != 1 {
			c.Header("WWW-Authenticate", "Bearer")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func uploadLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = shortener.DefaultMaxUploadBytes
	}
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload request too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func requestCostMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s completed in %s", c.Request.Method, c.Request.URL.Path, time.Since(start))
	}
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) createLink(c *gin.Context) {
	var request shortener.CreateLinkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	response, err := h.service.CreateLink(c.Request.Context(), request)
	if err != nil {
		if errors.Is(err, service.ErrInvalidURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "url must be an absolute http or https url"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, response)
}

func (h *Handler) createAssetLink(c *gin.Context) {
	form, err := c.MultipartForm()
	if form != nil {
		defer form.RemoveAll()
	}
	if err == nil {
		// Include any multipart epilogue in the request limit before uploading.
		_, err = io.Copy(io.Discard, c.Request.Body)
	}
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) || errors.Is(err, multipart.ErrMessageTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload request too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart body"})
		return
	}
	files := form.File["file"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	file := files[0]

	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer reader.Close()

	response, err := h.service.CreateObjectLink(c.Request.Context(), service.CreateObjectLinkRequest{
		Reader:      reader,
		FileName:    file.Filename,
		ContentType: file.Header.Get("Content-Type"),
		Size:        file.Size,
		Title:       strings.TrimSpace(c.PostForm("title")),
		Scope:       strings.TrimSpace(c.PostForm("scope")),
		TTLSeconds:  int64Form(c, "ttl_seconds"),
		MaxVisits:   int64Form(c, "max_visits"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, response)
}

func (h *Handler) redirect(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	link, err := h.service.Resolve(c.Request.Context(), code)
	if err != nil {
		writeLinkError(c, err)
		return
	}

	c.Redirect(http.StatusFound, link.URL)
}

func (h *Handler) getLinkInfo(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))

	link, err := h.service.GetLinkInfo(c.Request.Context(), code)
	if err != nil {
		writeLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, link)
}

func (h *Handler) getLinksInfo(c *gin.Context) {
	info, err := h.service.ListLinks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *Handler) deleteLinkInfo(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	info, err := h.service.DeleteLinkInfo(c.Request.Context(), code)
	if err != nil {
		writeLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func writeLinkError(c *gin.Context, err error) {
	status := http.StatusNotFound
	message := "link not found"
	if errors.Is(err, service.ErrLinkExpired) {
		status = http.StatusGone
		message = "link expired"
	}
	if errors.Is(err, service.ErrVisitsExhausted) {
		status = http.StatusGone
		message = "link max visits exhausted"
	}
	if status == http.StatusNotFound && err.Error() != message {
		status = http.StatusInternalServerError
		message = err.Error()
	}
	c.JSON(status, gin.H{"error": message})
}

func int64Form(c *gin.Context, key string) int64 {
	value := strings.TrimSpace(c.PostForm(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
