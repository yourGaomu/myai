package handle

import (
	"errors"
	"log"
	"myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/service"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *service.Service
}

func NewRouter(service *service.Service) *gin.Engine {
	handler := &Handler{service: service}

	router := gin.New()
	router.Use(gin.Recovery(), corsMiddleware(), requestCostMiddleware())
	router.GET("/health", handler.health)
	router.POST("/api/links", handler.createLink)
	router.POST("/api/assets", handler.createAssetLink)
	router.GET("/s/:code", handler.redirect)
	router.GET("/api/links/:code", handler.getLinkInfo)
	router.GET("/api/links", handler.getLinksInfo)
	router.DELETE("/api/links/:code", handler.deleteLinkInfo)
	return router

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
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

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
