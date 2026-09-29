package api

import (
	"net/http"
	"strconv"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/example/ab-platform/internal/assignment"
	"github.com/example/ab-platform/internal/storage"
)

type Server struct {
	store storage.Store
}

type decideRequest struct {
	Subject storage.Subject `json:"subject" binding:"required"`
	LayerID string          `json:"layer_id"`
	Record  bool            `json:"record"`
	Expose  bool            `json:"expose"`
}

type decideResponse struct {
	Decision   *assignment.Decision `json:"decision"`
	Assignment *storage.Assignment  `json:"assignment,omitempty"`
	ExposureID string               `json:"exposure_id,omitempty"`
	Recorded   bool                 `json:"recorded"`
	Exposed    bool                 `json:"exposed"`
}

type simulationUser struct {
	UserID     string `json:"user_id"`
	Registered bool   `json:"registered"`
	Country    string `json:"country"`
	Plan       string `json:"plan"`
}

type simulateRequest struct {
	LayerID string           `json:"layer_id"`
	Count   int              `json:"count"`
	Users   []simulationUser `json:"users"`
}

type simulateResponse struct {
	Count       int                   `json:"count"`
	Groups      map[string]int        `json:"groups"`
	Rejections  map[string]int        `json:"rejections"`
	Percentages map[string]float64    `json:"percentages"`
	Samples     []assignment.Decision `json:"samples,omitempty"`
}

func NewServer(store storage.Store) *Server {
	return &Server{store: store}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:4200"},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
	}))
	api := r.Group("/api")
	{
		api.GET("/health", s.health)
		api.GET("/config", s.getConfig)
		api.PUT("/config/layers/:id", s.upsertLayer)
		api.PUT("/config/experiments/:id", s.upsertExperiment)
		api.PUT("/config/variants/:id", s.upsertVariant)
		api.PUT("/config/whitelists/:id", s.upsertWhitelist)
		api.POST("/decide", s.decide)
		api.POST("/simulate", s.simulate)
		api.GET("/stats", s.stats)
	}
	return r
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "storage": "configured"})
}

func (s *Server) getConfig(c *gin.Context) {
	cfg, err := s.store.Config()
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (s *Server) upsertLayer(c *gin.Context) {
	var input storage.Layer
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.ID = c.Param("id")
	if err := s.store.UpsertLayer(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, input)
}

func (s *Server) upsertExperiment(c *gin.Context) {
	var input storage.Experiment
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.ID = c.Param("id")
	if err := s.store.UpsertExperiment(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, input)
}

func (s *Server) upsertVariant(c *gin.Context) {
	var input storage.Variant
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.ID = c.Param("id")
	if err := s.store.UpsertVariant(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, input)
}

func (s *Server) upsertWhitelist(c *gin.Context) {
	var input storage.Whitelist
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.ID = c.Param("id")
	if err := s.store.UpsertWhitelist(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, input)
}

func (s *Server) decide(c *gin.Context) {
	var req decideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.LayerID == "" {
		req.LayerID = "checkout_layer"
	}
	if req.Subject.Source == "" {
		req.Subject.Source = "synthetic"
	}
	cfg, err := s.store.Config()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	d, err := assignment.Evaluate(cfg, req.Subject, req.LayerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp := decideResponse{Decision: d}
	if req.Record {
		recorded, err := s.persistDecision(req.Subject, d, req.Expose)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "decision": d})
			return
		}
		resp.Assignment = &recorded.Assignment
		resp.ExposureID = recorded.ExposureID
		resp.Recorded, resp.Exposed = true, recorded.ExposureID != ""
	}
	c.JSON(http.StatusOK, resp)
}

type persisted struct {
	Assignment storage.Assignment
	ExposureID string
}

func (s *Server) persistDecision(subject storage.Subject, d *assignment.Decision, expose bool) (persisted, error) {
	a := storage.Assignment{
		UserID:       d.UserID,
		LayerID:      d.Layer.ID,
		Bucket:       d.Bucket,
		Status:       d.Status,
		RejectReason: d.RejectReason,
		Source:       subject.Source,
	}
	if d.Experiment != nil {
		a.ExperimentID = d.Experiment.ID
	}
	if d.Variant != nil {
		a.VariantID = d.Variant.ID
		a.VariantBucket = d.VariantBucket
	}
	if d.Whitelist != nil {
		a.OverrideReason = d.Whitelist.Reason
	}
	saved, err := s.store.UpsertAssignment(a)
	if err != nil {
		return persisted{}, err
	}
	result := persisted{Assignment: saved}
	// Exposures are deliberately separate from assignment and written only for
	// eligible, assigned users. Rejected users never enter exposure metrics.
	if expose && d.Status == assignment.StatusAssigned {
		exposure := storage.Exposure{
			ID:           uuid.NewString(),
			AssignmentID: saved.ID,
			UserID:       d.UserID,
			ExperimentID: d.Experiment.ID,
			VariantID:    d.Variant.ID,
			Source:       subject.Source,
		}
		if d.Whitelist != nil {
			exposure.OverrideReason = d.Whitelist.Reason
		}
		if err := s.store.InsertExposure(&exposure); err != nil {
			return persisted{}, err
		}
		result.ExposureID = exposure.ID
	}
	return result, nil
}

func (s *Server) simulate(c *gin.Context) {
	var req simulateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.LayerID == "" {
		req.LayerID = "checkout_layer"
	}
	if req.Count <= 0 && len(req.Users) == 0 {
		req.Count = 1000
	}
	cfg, err := s.store.Config()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	users := req.Users
	if len(users) == 0 {
		users = syntheticUsers(req.Count)
	}
	resp := simulateResponse{
		Count:       len(users),
		Groups:      map[string]int{},
		Rejections:  map[string]int{},
		Percentages: map[string]float64{},
	}
	for i, u := range users {
		subject := storage.Subject{
			UserID: u.UserID, Registered: u.Registered, Country: u.Country,
			Plan: u.Plan, Source: "synthetic-simulation",
		}
		d, err := assignment.Evaluate(cfg, subject, req.LayerID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if i < 6 {
			resp.Samples = append(resp.Samples, *d)
		}
		if d.Status == assignment.StatusAssigned {
			group := d.Experiment.ID + "/" + d.Variant.Name
			if d.Whitelist != nil {
				group = "whitelist:" + group
			}
			resp.Groups[group]++
		} else {
			resp.Rejections[d.RejectReason]++
		}
	}
	for group, n := range resp.Groups {
		resp.Percentages[group] = float64(n) * 100 / float64(resp.Count)
	}
	for reason, n := range resp.Rejections {
		resp.Percentages[reason] = float64(n) * 100 / float64(resp.Count)
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) stats(c *gin.Context) {
	source := c.Query("source")
	stats, err := s.store.Stats(source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func syntheticUsers(count int) []simulationUser {
	users := make([]simulationUser, count)
	plans := []string{"free", "paid"}
	countries := []string{"CA", "US"}
	for i := range users {
		// Independent moduli keep country and plan from becoming accidentally
		// correlated with each other or the FNV allocation hash.
		users[i] = simulationUser{
			UserID:     "syn-batch-" + strconv.Itoa(i + 10000)[1:],
			Registered: i%10 != 0,
			Country:    countries[(i/2)%2],
			Plan:       plans[i%2],
		}
	}
	return users
}
