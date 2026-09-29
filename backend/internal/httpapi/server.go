package httpapi

import (
	"fmt"
	"net/http"

	"github.com/example/explainab/internal/db"
	"github.com/example/explainab/internal/experiment"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Server struct {
	Repo   *db.Repo
	Engine experiment.Engine
}

func NewServer(repo *db.Repo) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:4200", "http://127.0.0.1:4200"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{"Content-Type"},
	}))

	s := &Server{Repo: repo, Engine: experiment.NewEngine()}
	r.GET("/api/health", s.health)
	r.GET("/api/layers", s.listLayers)
	r.GET("/api/layers/:key", s.getLayer)
	r.POST("/api/layers", s.createLayer)
	r.POST("/api/layers/:key/evaluate", s.evaluateLayer)
	r.POST("/api/simulate", s.simulate)
	r.GET("/api/stats", s.stats)
	return r
}

func (s *Server) health(c *gin.Context) {
	if err := s.Repo.DB.PingContext(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) listLayers(c *gin.Context) {
	layers, err := s.Repo.ListLayers(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"layers": layers})
}

func (s *Server) getLayer(c *gin.Context) {
	layer, err := s.Repo.GetLayerByKey(c.Request.Context(), c.Param("key"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, layer)
}

func (s *Server) createLayer(c *gin.Context) {
	var layer experiment.Layer
	if err := c.ShouldBindJSON(&layer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := s.Repo.CreateLayer(c.Request.Context(), layer)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Server) evaluateLayer(c *gin.Context) {
	var input experiment.UserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := experiment.ValidateID(input.UserID, "user id"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_USER_ID", "error": err.Error()})
		return
	}
	layer, err := s.Repo.GetLayerByKey(c.Request.Context(), c.Param("key"))
	if err != nil {
		respondError(c, err)
		return
	}
	decision := s.Engine.Evaluate(input, layer)
	decision, err = s.Repo.PersistAssignmentAndExposure(c.Request.Context(), input, layer, decision)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, decision)
}

type BatchRequest struct {
	LayerKey string `json:"layer_key"`
	N        int    `json:"n"`
	Country  string `json:"country"`
	Seed     int64  `json:"seed"`
}

type BatchSummary struct {
	Total        int64                 `json:"total"`
	ByStatus     map[string]int64      `json:"by_status"`
	ByExperiment map[string]int64      `json:"by_experiment"`
	ByVariant    map[string]int64      `json:"by_variant"`
	BySource     map[string]int64      `json:"by_source"`
	Decisions    []experiment.Decision `json:"decisions"`
}

func (r BatchRequest) SeedOrDefault() int64 {
	if r.Seed == 0 {
		return 20260929
	}
	return r.Seed
}

func (s *Server) simulate(c *gin.Context) {
	var req BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.N <= 0 || req.N > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_N", "error": "n must be between 1 and 20000"})
		return
	}
	layer, err := s.Repo.GetLayerByKey(c.Request.Context(), req.LayerKey)
	if err != nil {
		respondError(c, err)
		return
	}
	country := req.Country
	if country == "" {
		country = "US"
	}
	summary := BatchSummary{
		ByStatus:     map[string]int64{},
		ByExperiment: map[string]int64{},
		ByVariant:    map[string]int64{},
		BySource:     map[string]int64{},
		Decisions:    make([]experiment.Decision, 0, min(req.N, 20)),
	}
	for i := 0; i < req.N; i++ {
		id := fmt.Sprintf("synthetic-seed%06d-%06d", req.SeedOrDefault(), i)
		user := experiment.UserInput{
			UserID:         id,
			Known:          true,
			Country:        country,
			Registered:     true,
			AccountAgeDays: 30,
			Persist:        false,
			RecordExposure: false,
		}
		d := s.Engine.Evaluate(user, layer)
		summary.Total++
		summary.ByStatus[d.Status]++
		if d.ExperimentKey != "" {
			summary.ByExperiment[d.ExperimentKey]++
		}
		if d.VariantKey != "" {
			summary.ByVariant[d.ExperimentKey+":"+d.VariantKey]++
			summary.BySource[d.Source]++
		}
		if len(summary.Decisions) < 20 {
			summary.Decisions = append(summary.Decisions, d)
		}
	}
	c.JSON(http.StatusOK, summary)
}

func (s *Server) stats(c *gin.Context) {
	rows, err := s.Repo.Stats(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": rows})
}

func respondError(c *gin.Context, err error) {
	apiErr, ok := db.AsAPIError(err)
	if !ok {
		apiErr = db.APIError{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: err.Error()}
	}
	c.JSON(apiErr.Status, gin.H{"code": apiErr.Code, "error": apiErr.Message})
}
