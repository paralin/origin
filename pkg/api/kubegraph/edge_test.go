package kubegraph

import (
	"fmt"
	"testing"

	"github.com/gonum/graph"

	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/runtime"

	osgraph "github.com/openshift/origin/pkg/api/graph"
	kubegraph "github.com/openshift/origin/pkg/api/kubegraph/nodes"
)

type objectifier interface {
	Object() interface{}
}

func TestNamespaceEdgeMatching(t *testing.T) {
	g := osgraph.New()

	fn := func(namespace string, g osgraph.Interface) {
		pod := &kapi.Pod{}
		pod.Namespace = namespace
		pod.Name = "the-pod"
		pod.Labels = map[string]string{"a": "1"}
		kubegraph.EnsurePodNode(g, pod)

		rc := &kapi.ReplicationController{}
		rc.Namespace = namespace
		rc.Name = "the-rc"
		rc.Spec.Selector = map[string]string{"a": "1"}
		kubegraph.EnsureReplicationControllerNode(g, rc)

		svc := &kapi.Service{}
		svc.Namespace = namespace
		svc.Name = "the-svc"
		svc.Spec.Selector = map[string]string{"a": "1"}
		kubegraph.EnsureServiceNode(g, svc)
	}

	fn("ns", g)
	fn("other", g)
	AddAllExposedPodEdges(g)
	AddAllExposedPodTemplateSpecEdges(g)
	AddAllManagedByRCPodEdges(g)

	for _, edge := range g.Edges() {
		nsTo, err := namespaceFor(edge.To())
		if err != nil {
			t.Fatal(err)
		}
		nsFrom, err := namespaceFor(edge.From())
		if err != nil {
			t.Fatal(err)
		}
		if nsFrom != nsTo {
			t.Errorf("edge %#v crosses namespace: %s %s", edge, nsFrom, nsTo)
		}
	}
}

func namespaceFor(node graph.Node) (string, error) {
	obj := node.(objectifier).Object()
	switch t := obj.(type) {
	case runtime.Object:
		meta, err := kapi.ObjectMetaFor(t)
		if err != nil {
			return "", err
		}
		return meta.Namespace, nil
	case *kapi.PodSpec:
		return node.(*kubegraph.PodSpecNode).Namespace, nil
	case *kapi.ReplicationControllerSpec:
		return node.(*kubegraph.ReplicationControllerSpecNode).Namespace, nil
	default:
		return "", fmt.Errorf("unknown object: %#v", obj)
	}
}

func TestSecretEdges(t *testing.T) {
	sa := &kapi.ServiceAccount{}
	sa.Namespace = "ns"
	sa.Name = "shultz"
	sa.Secrets = []kapi.ObjectReference{{Name: "i-know-nothing"}, {Name: "missing"}}

	secret1 := &kapi.Secret{}
	secret1.Namespace = "ns"
	secret1.Name = "i-know-nothing"

	pod := &kapi.Pod{}
	pod.Namespace = "ns"
	pod.Name = "the-pod"
	pod.Spec.Volumes = []kapi.Volume{{Name: "rose", VolumeSource: kapi.VolumeSource{Secret: &kapi.SecretVolumeSource{SecretName: "i-know-nothing"}}}}

	g := osgraph.New()
	saNode := kubegraph.EnsureServiceAccountNode(g, sa)
	secretNode := kubegraph.EnsureSecretNode(g, secret1)
	podNode := kubegraph.EnsurePodNode(g, pod)

	AddAllMountableSecretEdges(g)
	AddAllMountedSecretEdges(g)

	if edge := g.Edge(saNode, secretNode); edge == nil {
		t.Errorf("edge missing")
	} else {
		if !g.EdgeKinds(edge).Has(MountableSecretEdgeKind) {
			t.Errorf("expected %v, got %v", MountableSecretEdgeKind, edge)
		}
	}

	podSpecNodes := g.SuccessorNodesByNodeAndEdgeKind(podNode, kubegraph.PodSpecNodeKind, osgraph.ContainsEdgeKind)
	if len(podSpecNodes) != 1 {
		t.Fatalf("wrong number of podspecs: %v", podSpecNodes)
	}

	if edge := g.Edge(podSpecNodes[0], secretNode); edge == nil {
		t.Errorf("edge missing")
	} else {
		if !g.EdgeKinds(edge).Has(MountedSecretEdgeKind) {
			t.Errorf("expected %v, got %v", MountedSecretEdgeKind, edge)
		}
	}
}
