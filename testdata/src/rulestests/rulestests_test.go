package rulestests

import (
	"log"
	"testing"
)

func TestWithLog(t *testing.T) {
	log.Printf("created %s", "Email") // want `no-pii-in-logs: this log includes personal data \(0\.97\)`
}
