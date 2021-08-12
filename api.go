package volm

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

// Config configures the API
type Config struct {
	Service string
	Version string
	Log     *zap.Logger
	Cs      *kubernetes.Clientset
}

// API is primary object implementing the core API methods
// and HTTP handlers
type API struct {
	*Config
	LogErrors prometheus.Counter
}

// NewApi constructs an API object and populates it with
// configuration along with setting defaults where required.
func NewApi(cfg *Config) (*API, error) {
	a := &API{Config: cfg}

	// default logger if none specified
	if a.Log == nil {
		zapCfg := zap.NewProductionConfig()
		logger, err := zapCfg.Build()
		if err != nil {
			os.Exit(1)
		}

		a.Log = logger
	}

	return a, nil
}

// OkHandler is provided for created a default slash route for the
// HTTP API and returns basic version, node and service name.
func (a *API) OkHandler(version string, mode string, service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version": version, "mode": mode, "service": service})
	}
}
