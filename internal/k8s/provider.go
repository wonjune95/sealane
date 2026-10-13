package k8s

import (
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
)

// ErrResourceVersionRequired is returned by Update when the object lacks a
// resourceVersion (prevents lost updates). Handlers map it to 400.
var ErrResourceVersionRequired = errors.New("resourceVersion is required for update")

// BadRequestError marks a client input error (e.g. malformed JSON) → HTTP 400.
type BadRequestError struct{ Err error }

func (e *BadRequestError) Error() string { return e.Err.Error() }

func badRequest(err error) error { return &BadRequestError{Err: err} }

func createOpts(dryRun bool) metav1.CreateOptions {
	if dryRun {
		return metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}
	}
	return metav1.CreateOptions{}
}

func updateOpts(dryRun bool) metav1.UpdateOptions {
	if dryRun {
		return metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}
	}
	return metav1.UpdateOptions{}
}

func age(t metav1.Time) string {
	if t.IsZero() {
		return ""
	}
	return duration.HumanDuration(time.Since(t.Time))
}
