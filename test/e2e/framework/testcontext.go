package framework

import (
	"k8s.io/api/core/v1"
)

var TestContext TestContextType

type DeleteTestingNSPolicyType string

var (
	DeleteTestingNSPolicyAlways    DeleteTestingNSPolicyType = "Always"
	DeleteTestingNSPolicyOnSuccess DeleteTestingNSPolicyType = "OnSuccess"
	DeleteTestingNSPolicyNever     DeleteTestingNSPolicyType = "Never"
)

type CreateTestingNSFn func(f *Framework, name string, labels map[string]string) (*v1.Namespace, error)
type DeleteTestingNSFn func(f *Framework, ns *v1.Namespace) error

type TestContextType struct {
	KubeConfigPath        string
	CreateTestingNS       CreateTestingNSFn
	DeleteTestingNS       DeleteTestingNSFn
	DeleteTestingNSPolicy DeleteTestingNSPolicyType
}
