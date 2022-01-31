//go:build extended
// +build extended

package monitoring_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/resty.v1"
	"zotregistry.io/zot/pkg/api"
	"zotregistry.io/zot/pkg/api/config"
	extconf "zotregistry.io/zot/pkg/extensions/config"
	"zotregistry.io/zot/pkg/extensions/monitoring"
	. "zotregistry.io/zot/test"
)

func TestExtensionMetrics(t *testing.T) {
	Convey("Make a new controller with explicitly enabled metrics", t, func() {
		port := GetFreePort()
		baseURL := GetBaseURL(port)
		conf := config.New()
		conf.HTTP.Port = port

		rootDir, err := ioutil.TempDir("", "metrics-test")
		if err != nil {
			panic(err)
		}
		defer os.RemoveAll(rootDir)

		conf.Storage.RootDirectory = rootDir
		conf.Extensions = &extconf.ExtensionConfig{}
		conf.Extensions.Metrics = &extconf.MetricsConfig{
			Enable:     true,
			Prometheus: &extconf.PrometheusConfig{Path: "/metrics"},
		}

		ctlr := api.NewController(conf)
		So(ctlr, ShouldNotBeNil)

		go startServer(ctlr)
		defer stopServer(ctlr)
		WaitTillServerReady(baseURL)

		// improve code coverage
		ctlr.Metrics.SendMetric(baseURL)
		ctlr.Metrics.ForceSendMetric(baseURL)

		So(ctlr.Metrics.IsEnabled(), ShouldBeTrue)
		So(ctlr.Metrics.ReceiveMetrics(), ShouldBeNil)

		monitoring.ObserveHTTPRepoLatency(ctlr.Metrics,
			"/v2/alpine/blobs/uploads/299148f0-0e32-4830-90d2-a3fa744137d9", time.Millisecond)
		monitoring.IncDownloadCounter(ctlr.Metrics, "alpine")
		monitoring.IncUploadCounter(ctlr.Metrics, "alpine")

		err = CopyFiles("../../../test/data/zot-test", path.Join(rootDir, "alpine"))
		if err != nil {
			panic(err)
		}
		monitoring.SetStorageUsage(ctlr.Metrics, rootDir, "alpine")

		monitoring.ObserveStorageLockLatency(ctlr.Metrics, time.Millisecond, rootDir, "RWLock")

		resp, err := resty.R().Get(baseURL + "/metrics")
		So(err, ShouldBeNil)
		So(resp, ShouldNotBeNil)
		So(resp.StatusCode(), ShouldEqual, http.StatusOK)

		respStr := string(resp.Body())
		So(respStr, ShouldContainSubstring, "zot_repo_downloads_total{repo=\"alpine\"} 1")
		So(respStr, ShouldContainSubstring, "zot_repo_uploads_total{repo=\"alpine\"} 1")
		So(respStr, ShouldContainSubstring, "zot_repo_storage_bytes{repo=\"alpine\"}")
		So(respStr, ShouldContainSubstring, "zot_storage_lock_latency_seconds_bucket")
		So(respStr, ShouldContainSubstring, "zot_storage_lock_latency_seconds_sum")
		So(respStr, ShouldContainSubstring, "zot_storage_lock_latency_seconds_bucket")
	})
	Convey("Make a new controller with disabled metrics extension", t, func() {
		port := GetFreePort()
		baseURL := GetBaseURL(port)
		conf := config.New()
		conf.HTTP.Port = port

		rootDir, err := ioutil.TempDir("", "metrics-test")
		if err != nil {
			panic(err)
		}
		defer os.RemoveAll(rootDir)

		conf.Storage.RootDirectory = rootDir
		conf.Extensions = &extconf.ExtensionConfig{}
		conf.Extensions.Metrics = &extconf.MetricsConfig{Enable: false}

		ctlr := api.NewController(conf)
		So(ctlr, ShouldNotBeNil)

		go startServer(ctlr)
		defer stopServer(ctlr)
		WaitTillServerReady(baseURL)

		So(ctlr.Metrics.IsEnabled(), ShouldBeFalse)

		resp, err := resty.R().Get(baseURL + "/metrics")
		So(err, ShouldBeNil)
		So(resp, ShouldNotBeNil)
		So(resp.StatusCode(), ShouldEqual, http.StatusNotFound)
	})
}

func startServer(c *api.Controller) {
	// this blocks
	if err := c.Run(); err != nil {
		return
	}
}

func stopServer(c *api.Controller) {
	ctx := context.Background()
	_ = c.Server.Shutdown(ctx)
}