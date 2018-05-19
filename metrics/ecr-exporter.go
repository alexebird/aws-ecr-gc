package metrics

import (
	"flag"
	"fmt"
	"log"
	//"math"
	"net/http"
	"os"
	//"strconv"
	//"strings"
	"sync"
	//"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
)

const (
	namespace = "ecr"
)

var (
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last query of ECR successful",
		nil, nil,
	)
	imageCount = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "images"),
		"How many images are in the repository",
		[]string{"repository"}, nil,
	)
)

type Exporter struct {
	client     *ecr.ECR
	registryId *string
}

func NewExporter(registryId *string, region *string) (*Exporter, error) {
	// rely on env vars.
	client := ecr.New(session.New(&aws.Config{Region: region}))
	return &Exporter{
		client:     client,
		registryId: registryId,
	}, nil
}

// Describe implements Collector interface.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- imageCount
}

// Collect collects nomad metrics
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(
		up, prometheus.GaugeValue, 1,
	)

	reposInput := &ecr.DescribeRepositoriesInput{RegistryId: e.registryId}
	repoNames := make([]string, 0)

	err := e.client.DescribeRepositoriesPages(reposInput,
		func(page *ecr.DescribeRepositoriesOutput, lastPage bool) bool {
			for _, repo := range page.Repositories {
				repoNames = append(repoNames, *repo.RepositoryName)
			}
			return true
		})

	if err != nil {
		logError(err)
		return
	}

	//fmt.Println(repoNames)

	var w sync.WaitGroup
	for _, repo := range repoNames {
		w.Add(1)
		go func(repo string) {
			defer w.Done()
			//func(repo string) {
			imageCnt := 0

			imagesInput := &ecr.ListImagesInput{
				RepositoryName: &repo,
			}

			err := e.client.ListImagesPages(imagesInput,
				func(page *ecr.ListImagesOutput, lastPage bool) bool {
					imageCnt += len(page.ImageIds)
					return true
				})

			if err != nil {
				logError(err)
				return
			}

			//fmt.Println("image count", repo, imageCnt)

			ch <- prometheus.MustNewConstMetric(
				imageCount, prometheus.GaugeValue, float64(imageCnt), repo,
			)
		}(repo)
	}
	w.Wait()
}

func main() {

	var (
		showVersion   = flag.Bool("version", false, "Print version information.")
		listenAddress = flag.String("web.listen-address", ":8070", "Address to listen on for web interface and telemetry.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		region        = flag.String("aws.region", "", "The aws region")
		registryId    = flag.String("aws.registry-id", "", "The aws registry id")
	)
	flag.Parse()

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("ecr_exporter"))
		os.Exit(0)
	}

	log.Println("aws region:", *region)
	log.Println("aws registry id:", *registryId)

	exporter, err := NewExporter(registryId, region)
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>ECR Exporter</title></head>
             <body>
             <h1>ECR Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	log.Println("Listening on", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}

func logError(err error) {
	log.Println("error", err)
	return
}
