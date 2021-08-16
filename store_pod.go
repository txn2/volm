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

type PodStoreConfig struct {
	Namespace string
	Log       *zap.Logger
	Cs        *kubernetes.Clientset
}

type PodStore struct {
	*PodStoreConfig
	Stopper chan struct{}
	podMap  map[string]v1.Pod
	sync.Mutex
}

func NewPodStore(cfg *PodStoreConfig) (*PodStore, error) {
	ps := &PodStore{PodStoreConfig: cfg}

	if ps.Cs == nil {
		return nil, fmt.Errorf("must specify kubernetes.Clientset")
	}

	if ps.Log == nil {
		return nil, fmt.Errorf("must specify zap.Logger")
	}

	if ps.Namespace == "" {
		return nil, fmt.Errorf("must specify a Namespace")
	}

	ps.podMap = make(map[string]v1.Pod, 0)
	ps.Stopper = make(chan struct{})
	ps.PodWatch()

	return ps, nil
}

func (ps *PodStore) PodWatch() {
	factory := informers.NewSharedInformerFactoryWithOptions(ps.Cs, time.Second*60, informers.WithNamespace(ps.Namespace))
	informer := factory.Core().V1().Pods().Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			ps.AddPod(*pod)
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			ps.DeletePod(pod.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pod := newObj.(*v1.Pod)
			ps.AddPod(*pod)
		},
	})

	go informer.Run(ps.Stopper)
}

func (ps *PodStore) AddPod(pod v1.Pod) {
	ps.Lock()
	ps.Log.Info("AddPod", zap.String("name", pod.Name))
	ps.podMap[pod.Name] = pod
	ps.Unlock()
}

func (ps *PodStore) DeletePod(podName string) {
	ps.Lock()
	_, ok := ps.podMap[podName]
	if ok {
		ps.Log.Info("DeletePod", zap.String("name", podName))
		delete(ps.podMap, podName)
	}
	ps.Unlock()
}

func (ps *PodStore) GetPod(podName string) *v1.Pod {
	pod, ok := ps.podMap[podName]
	if ok {
		return &pod
	}
	return nil
}

func (ps *PodStore) GetPods() []v1.Pod {
	var pods []v1.Pod
	for _, p := range ps.podMap {
		pods = append(pods, p)
	}
	return pods
}
