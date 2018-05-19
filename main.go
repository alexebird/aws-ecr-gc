package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	//"github.com/alexebird/aws-ecr-gc/gc"
	"github.com/alexebird/aws-ecr-gc/model"
	"github.com/alexebird/aws-ecr-gc/registry"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	//awsecr "github.com/aws/aws-sdk-go/service/ecr"
	"github.com/davecgh/go-spew/spew"
	vault "github.com/hashicorp/vault/api"
	log "github.com/sirupsen/logrus"
)

type keepCountMap map[string]uint

func (k keepCountMap) String() string {
	var s []string
	for prefix, count := range k {
		s = append(s, fmt.Sprintf("%s=%d", prefix, count))
	}
	return "{" + strings.Join(s, ", ") + "}"
}

func (k keepCountMap) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected prefix=COUNT e.g. release=4")
	}
	prefix := parts[0]
	count, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return fmt.Errorf("expected N in %s=N to be non-negative integer", prefix)
	}
	k[prefix] = uint(count)
	return nil
}

func waitForIamCreds(fn func() bool) bool {
	wg := sync.WaitGroup{}
	wg.Add(1)
	timeout := 30 * time.Second
	c := make(chan struct{})

	go func() {
		for {
			if !fn() {
				time.Sleep(time.Second)
			} else {
				break
			}
		}
		wg.Done()
	}()

	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}

}

func main() {
	//var repo string
	//var deleteUntagged bool
	//keepCounts := keepCountMap{}
	//flag.StringVar(&region, "region", os.Getenv("AWS_DEFAULT_REGION"), "AWS region (defaults to AWS_DEFAULT_REGION in environment)")
	//flag.StringVar(&repo, "repo", "", "AWS ECR repository name")
	//flag.BoolVar(&deleteUntagged, "delete-untagged", deleteUntagged, "whether to delete untagged images")
	//flag.Var(&keepCounts, "keep", "map of image tag prefixes to how many to keep, e.g. --keep release=4 --keep build=8")
	//flag.Parse()
	//if region == "" || repo == "" {
	//flag.Usage()
	//os.Exit(2)
	//}

	ecr := registry.NewSession(awsConf())

	if os.Getenv("VAULT_TOKEN") != "" {
		waitFn := func() bool {
			_, err := ecr.Repositories()
			return err == nil
		}

		if waitForIamCreds(waitFn) {
			panic("timed out waiting for IAM creds")
		}
	}

	repos, err := ecr.Repositories()
	if err != nil {
		panic(err)
	}

	lvl, err := log.ParseLevel("debug")
	if err != nil {
		panic(err)
	}

	log.SetOutput(os.Stdout)
	log.SetLevel(lvl)

	// 1. collect images into some kind of data structure

	for _, repo := range repos {
		log.Debug(spew.Sdump(repo))
		images := ecr.Images(repo.RepositoryName)
		log.Debug(spew.Sdump(images))
	}

	// 2. delete untagged images
	// 3. delete images over the specified threshold (say around 900)
	// 4. read a list of regexes from a yaml config, match them against tags, keep N most recent of each regex.

	//spew.Dump(repos)

	//spew.Dump(ecr.Images("web-express"))
	//images, err := ecr.Images(repo)
	//if err != nil {
	//panic(err)
	//}
	//fmt.Printf("Total images in %s (%s): %d\n", repo, region, len(images))

	//gcParams := gc.Params{KeepCounts: keepCounts, DeleteUntagged: deleteUntagged}
	//deletionList := gc.ImagesToDelete(images, gcParams)
	//printImages("Images to delete", deletionList)
	//result, err := ecr.DeleteImages(repo, deletionList)
	//if err != nil {
	//panic(err)
	//}
	//printResult(result)
}

func printImages(heading string, images model.Images) {
	fmt.Printf("%s (%d)\n", heading, len(images))
	for _, img := range images {
		fmt.Printf(
			"  %s: %s... [%s]\n",
			img.PushedAt.Format("2006-01-02 15:04:05"),
			img.Digest[0:16],
			strings.Join(img.Tags, ", "),
		)
	}
}

func printResult(result *model.DeleteImagesResult) {
	fmt.Printf("Deleted (%d)\n", len(result.Deletions))
	for _, id := range result.Deletions {
		fmt.Printf("  %s... (%s)\n", id.Digest[0:16], id.Tag)
	}
	fmt.Printf("Failures (%d)\n", len(result.Failures))
	for _, f := range result.Failures {
		fmt.Printf("  %s... %s: %s\n", f.ID.Digest[0:16], f.Code, f.Reason)
	}
}

func awsConf() aws.Config {
	var creds *credentials.Credentials
	region := os.Getenv("ECR_REGION")
	conf := aws.Config{Region: &region}

	if os.Getenv("VAULT_TOKEN") != "" {
		creds = vaultAwsConf()
		_, err := creds.Get()
		if err != nil {
			panic(err)
		}
		conf = *conf.WithCredentials(creds)
	}

	return conf
}

func vaultAwsConf() *credentials.Credentials {
	vaultClient, err := vault.NewClient(vault.DefaultConfig())
	if err != nil {
		panic(err)
	}

	return credentials.NewCredentials(&VaultProvider{client: vaultClient})
}
