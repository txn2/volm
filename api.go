package volm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type VolumeInfo struct {
	Name             string                         `json:"name"`
	Labels           map[string]string              `json:"labels,omitempty"`
	Annotations      map[string]string              `json:"annotations,omitempty"`
	Status           v1.PersistentVolumeClaimStatus `json:"status"`
	Spec             v1.PersistentVolumeClaimSpec   `json:"spec"`
	Terminating      bool                           `json:"terminating"`
	TerminatingSince *metaV1.Time                   `json:"terminatingSince,omitempty"`
	UsedBy           []PodInfo                      `json:"usedBy"`
}

type PodInfo struct {
	Name             string            `json:"name"`
	Labels           map[string]string `json:"labels,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	Phase            v1.PodPhase       `json:"phase"`
	StartTime        *metaV1.Time      `json:"startTime"`
	Terminating      bool              `json:"terminating"`
	TerminatingSince *metaV1.Time      `json:"terminatingSince,omitempty"`
}

// Config configures the API
type Config struct {
	Service      string
	Version      string
	Log          *zap.Logger
	Cs           *kubernetes.Clientset
	PVCNamespace string
	PVCSelector  string
}

// API is primary object implementing the core API methods
// and HTTP handlers
type API struct {
	*Config
	LogErrors      prometheus.Counter
	PVCSelectorMap map[string]string
	PodStore       *PodStore
	PVCStore       *PVCStore
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

	a.PVCSelectorMap = map[string]string{}
	if a.PVCSelector != "" {
		kvs := strings.Split(a.PVCSelector, ",")
		for _, kv := range kvs {
			kv := strings.Split(kv, "=")
			if len(kv) < 2 {
				a.Log.Fatal("Malformed PVC selector")
				os.Exit(1)
			}

			a.PVCSelectorMap[kv[0]] = kv[1]
		}
	}

	podStore, err := NewPodStore(&PodStoreConfig{
		Namespace: a.PVCNamespace,
		Log:       a.Log,
		Cs:        a.Cs,
	})
	if err != nil {
		return a, err
	}

	a.PodStore = podStore

	pvcStore, err := NewPVCStore(&PVCStoreConfig{
		Namespace: a.PVCNamespace,
		Log:       a.Log,
		Cs:        a.Cs,
	})
	if err != nil {
		return a, err
	}

	a.PVCStore = pvcStore

	return a, nil
}

// IsNotFound returns true if the error is a errors.StatusError
// matching metaV1.StatusReasonNotFound this function allows us
// to log more critical errors and pass status information such
// at 404s and 500s through the REST API.
func IsNotFound(err error) bool {
	if statusError, isStatus := err.(*errors.StatusError); isStatus && statusError.Status().Reason == metaV1.StatusReasonNotFound {
		return true
	}

	return false
}

// OkHandler is provided for created a default slash route for the
// HTTP API and returns basic version, node and service name.
func (a *API) OkHandler(version string, mode string, service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version": version, "mode": mode, "service": service})
	}
}

func (a *API) ListPVCHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		pvcList, err := a.GetPVCList()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, pvcList)
	}
}

func (a *API) GetPVCList() ([]VolumeInfo, error) {
	vols := make([]VolumeInfo, 0)

	// get all pods
	pods := a.PodStore.GetPods()

	for _, pvc := range a.PVCStore.GetPVCs() {
		var terminating bool
		var terminatingSince *metaV1.Time

		selectorPass := true

		// ensure PVC meets selector criteria
		for k, v := range a.PVCSelectorMap {
			if _, ok := pvc.Labels[k]; !ok {
				selectorPass = false
			}

			if pvc.Labels[k] != v {
				selectorPass = false
			}
		}

		if !selectorPass {
			continue
		}

		podList, err := a.GetPodsInfoByPVC(pods, pvc.Name)
		if err != nil {
			return vols, err
		}

		// See https://github.com/kubernetes/kubernetes/issues/22839
		// on terminating status
		if pvc.DeletionTimestamp != nil {
			terminating = true
			terminatingSince = pvc.DeletionTimestamp
		}
		vols = append(vols, VolumeInfo{
			Name:             pvc.Name,
			Labels:           pvc.Labels,
			Annotations:      pvc.Annotations,
			Status:           pvc.Status,
			Spec:             pvc.Spec,
			Terminating:      terminating,
			TerminatingSince: terminatingSince,
			UsedBy:           podList,
		})
	}

	return vols, nil
}

func (a *API) GetPVCHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		pvc, err := a.GetPVC(c.Param("name"))
		if err != nil && err.Error() == "not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, pvc)
	}
}

func (a *API) GetPVC(name string) (VolumeInfo, error) {
	volInfo := VolumeInfo{}

	pvc := a.PVCStore.GetPVC(name)
	if pvc == nil {
		return volInfo, fmt.Errorf("not found")
	}

	// ensure PVC meets selector criteria
	for k, v := range a.PVCSelectorMap {
		if _, ok := pvc.Labels[k]; !ok {
			return volInfo, fmt.Errorf("PVC labels does not contain key %s", k)
		}

		if pvc.Labels[k] != v {
			return volInfo, fmt.Errorf("PVC label %s does not contain value %s", k, v)
		}
	}

	volInfo.Name = pvc.Name
	volInfo.Labels = pvc.Labels
	volInfo.Annotations = pvc.Annotations
	volInfo.Spec = pvc.Spec
	volInfo.Status = pvc.Status

	// See https://github.com/kubernetes/kubernetes/issues/22839
	// on terminating status
	if pvc.DeletionTimestamp != nil {
		volInfo.Terminating = true
		volInfo.TerminatingSince = pvc.DeletionTimestamp
	}

	pods := a.PodStore.GetPods()
	podList, err := a.GetPodsInfoByPVC(pods, pvc.Name)
	if err != nil {
		return VolumeInfo{}, err
	}

	volInfo.UsedBy = podList

	return volInfo, err
}

func (a *API) DeletePVCHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		err := a.DeletePVC(c.Param("name"))
		if IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": true})
	}
}

func (a *API) DeletePVC(name string) error {
	ctx := context.Background()

	pvcClient := a.Cs.CoreV1().PersistentVolumeClaims(a.PVCNamespace)

	pvc, err := pvcClient.Get(ctx, name, metaV1.GetOptions{})
	if IsNotFound(err) {
		return err
	}
	if err != nil {
		a.Log.Error("GetPVC got error invoking pvcClient.Get", zap.Error(err))
		return err
	}

	// ensure PVC meets selector criteria
	for k, v := range a.PVCSelectorMap {
		if _, ok := pvc.Labels[k]; !ok {
			return fmt.Errorf("PVC labels does not contain key %s", k)
		}

		if pvc.Labels[k] != v {
			return fmt.Errorf("PVC label %s does not contain value %s", k, v)
		}
	}

	err = pvcClient.Delete(ctx, name, metaV1.DeleteOptions{})
	if err != nil {
		a.Log.Error("DeletePVC got error invoking pvcClient.Delete", zap.Error(err))
		return err
	}

	return nil
}

func (a *API) GetPodsInfoByPVC(pods []v1.Pod, pvcName string) ([]PodInfo, error) {
	var podInfoList []PodInfo

	for _, pod := range pods {
		for _, v := range pod.Spec.Volumes {
			if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvcName {
				var terminating bool
				var terminatingSince *metaV1.Time

				if pod.DeletionTimestamp != nil {
					terminating = true
					terminatingSince = pod.DeletionTimestamp
				}

				podInfoList = append(podInfoList, PodInfo{
					Name:             pod.Name,
					Labels:           pod.Labels,
					Annotations:      pod.Annotations,
					Phase:            pod.Status.Phase,
					StartTime:        pod.Status.StartTime,
					Terminating:      terminating,
					TerminatingSince: terminatingSince,
				})
			}
		}
	}

	return podInfoList, nil
}
