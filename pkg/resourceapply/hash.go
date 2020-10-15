package resourceapply

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"

	"github.com/tnozicka/openshift-acme/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

func HashObjectsOrDie(objs ...runtime.Object) string {
	hasher := sha512.New()

	for _, obj := range objs {
		data, err := json.Marshal(obj)
		if err != nil {
			panic(err)
		}

		_, err = hasher.Write(data)
		if err != nil {
			panic(err)
		}
	}

	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}

func SetHashOrDie(obj runtime.Object) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		panic(err)
	}

	annotations := accessor.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations[api.ManagedDataHash] = "" // produce the same hash for objects already having the annotation
	annotations[api.ManagedDataHash] = HashObjectsOrDie(obj)

	accessor.SetAnnotations(annotations)
}
