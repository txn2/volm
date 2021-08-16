package volm

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type PVCStoreConfig struct {
	Namespace string
	Log       *zap.Logger
	Cs        *kubernetes.Clientset
}

type PVCStore struct {
	*PVCStoreConfig
	Stopper chan struct{}
	pvcMap  map[string]v1.PersistentVolumeClaim
	sync.Mutex
}

func NewPVCStore(cfg *PVCStoreConfig) (*PVCStore, error) {
	ps := &PVCStore{PVCStoreConfig: cfg}

	if ps.Cs == nil {
		return nil, fmt.Errorf("must specify kubernetes.Clientset")
	}

	if ps.Log == nil {
		return nil, fmt.Errorf("must specify zap.Logger")
	}

	if ps.Namespace == "" {
		return nil, fmt.Errorf("must specify a Namespace")
	}

	ps.pvcMap = make(map[string]v1.PersistentVolumeClaim, 0)
	ps.Stopper = make(chan struct{})
	ps.PVCWatch()

	return ps, nil
}

func (pvcs *PVCStore) PVCWatch() {
	factory := informers.NewSharedInformerFactoryWithOptions(pvcs.Cs, time.Second*60, informers.WithNamespace(pvcs.Namespace))
	informer := factory.Core().V1().PersistentVolumeClaims().Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pvc := obj.(*v1.PersistentVolumeClaim)
			pvcs.AddPVC(*pvc)
		},
		DeleteFunc: func(obj interface{}) {
			pvc := obj.(*v1.PersistentVolumeClaim)
			pvcs.DeletePVC(pvc.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pvc := oldObj.(*v1.PersistentVolumeClaim)
			pvcs.AddPVC(*pvc)
		},
	})

	go informer.Run(pvcs.Stopper)
}

func (pvcs *PVCStore) AddPVC(pvc v1.PersistentVolumeClaim) {
	pvcs.Lock()
	pvcs.Log.Info("AddPVC", zap.String("name", pvc.Name))
	pvcs.pvcMap[pvc.Name] = pvc
	pvcs.Unlock()
}

func (pvcs *PVCStore) DeletePVC(podName string) {
	pvcs.Lock()
	_, ok := pvcs.pvcMap[podName]
	if ok {
		pvcs.Log.Info("DeletePVC", zap.String("name", podName))
		delete(pvcs.pvcMap, podName)
	}
	pvcs.Unlock()
}

func (pvcs *PVCStore) GetPVC(pvcName string) *v1.PersistentVolumeClaim {
	pvc, ok := pvcs.pvcMap[pvcName]
	if ok {
		return &pvc
	}
	return nil
}

func (pvcs *PVCStore) GetPVCs() []v1.PersistentVolumeClaim {
	var pvcList []v1.PersistentVolumeClaim
	for _, p := range pvcs.pvcMap {
		pvcList = append(pvcList, p)
	}
	return pvcList
}
