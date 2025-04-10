package bgp

import (
	"fmt"
	"math"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

func statusErrorWithMessage(msg string, params ...interface{}) metav1.Status {
	return metav1.Status{
		Message: fmt.Sprintf(msg, params...),
		Status:  metav1.StatusFailure,
	}
}

func ConvertBgpPolicy(object *unstructured.Unstructured, toVersion string) (*unstructured.Unstructured, metav1.Status) {
	convertedObject := object.DeepCopy()
	fromVersion := object.GetAPIVersion()
	if toVersion == fromVersion {
		return nil, statusErrorWithMessage("conversion from a version to itself should not call the webhook: %s", toVersion)
	}

	klog.V(2).InfoS("Converting CRD for BGPPolicy", "fromVersion", fromVersion, "toVersion", toVersion)
	switch fromVersion {
	case "crd.antrea.io/v1alpha1":
		switch toVersion {
		case "crd.antrea.io/v1alpha2":
			// Nothing to do; a valid v1alpha1 object is also a valid v1alpha2 object.
			break
		default:
			return nil, statusErrorWithMessage("unexpected conversion fromVersion %q to toVersion %q", fromVersion, toVersion)
		}
	case "crd.antrea.io/v1alpha2":
		switch toVersion {
		case "crd.antrea.io/v1alpha1":
			localAsn, _, _ := unstructured.NestedInt64(convertedObject.Object, "spec", "localASN")
			if localAsn > math.MaxUint16 {
				// If confederation id is higher than we can represent we set it to 0.
				// Not ideal but needed as conversion success is obligatory.
				unstructured.SetNestedField(convertedObject.Object, int64(0), "spec", "localASN")
			}

			bgpPeers, _, _ := unstructured.NestedSlice(convertedObject.Object, "spec", "bgpPeers")
			for _, r := range bgpPeers {
				bgpPeer, ok := r.(map[string]interface{})
				if !ok {
					return nil, statusErrorWithMessage("failed to convert bgpPeer")
				}

				peerAsn, _ := bgpPeer["asn"].(int64)
				if peerAsn > math.MaxUint16 {
					bgpPeer["asn"] = int64(0)
				}
			}

			unstructured.SetNestedSlice(convertedObject.Object, bgpPeers, "spec", "bgpPeers")

			confederation, ok, _ := unstructured.NestedMap(convertedObject.Object, "spec", "confederation")
			if ok {
				// While removing the invalid confederation config would be easiest,
				// we instead convert it, preserving 32-bit ASNs as 0.
				// This is better than returning valid-looking objects that's missing configuration.
				confederationId, _ := confederation["identifier"].(int64)
				if confederationId > math.MaxUint16 {
					// Deleting instead of setting to 0 as the field has omitempty
					delete(confederation, "identifier")
				}

				memberASNs, _ := confederation["memberASNs"].([]interface{})
				if len(memberASNs) == 0 {
					// Deleting as the field has omitempty
					delete(confederation, "memberASNs")
				}

				for i := range memberASNs {
					memberAsn := memberASNs[i].(int64)
					if memberAsn > math.MaxUint16 {
						//
						memberASNs[i] = int64(0)
					}
				}

				unstructured.SetNestedMap(convertedObject.Object, confederation, "spec", "confederation")
			}

		default:
			return nil, statusErrorWithMessage("unexpected conversion fromVersion %q to toVersion %q", fromVersion, toVersion)
		}
	}

	return convertedObject, metav1.Status{
		Status: metav1.StatusSuccess,
	}
}
