package ecrmirror

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/distribution/distribution/v3/registry/storage/driver"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"

	storagemiddleware "github.com/distribution/distribution/v3/registry/storage/driver/middleware"
	"github.com/sirupsen/logrus"
)

// init registers the ecr layerHandler backend.
func init() {
	if err := storagemiddleware.Register("ecr", newEcrFetcher); err != nil {
		logrus.Errorf("failed to register ecr middleware: %v", err)
	}
}

var _ storagedriver.StorageDriver = &ecrFetcher{}

type ecrFetcher struct {
	driver.StorageDriver
	remote string
	local  string
}

func newEcrFetcher(ctx context.Context, base driver.StorageDriver, options map[string]interface{}) (driver.StorageDriver, error) {
	remote, _ := options["remote"].(string)
	local, _ := options["local"].(string)
	return &ecrFetcher{
		StorageDriver: base,
		remote:        remote,
		local:         local,
	}, nil
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *ecrFetcher) GetContent(ctx context.Context, path string) ([]byte, error) {
	out, err := d.StorageDriver.GetContent(ctx, path)
	if err == nil {
		return out, nil
	}

	// If the path is not found, we will try to pull the image from ECR
	// e.g. /docker/registry/v2/repositories/packoff/cv/_manifests/tags/1.0.0/current/link
	regex := regexp.MustCompile(`^/docker/registry/v2/repositories/(.+)/_manifests/tags/(.+)/current/link$`)
	matches := regex.FindStringSubmatch(path)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid path: %s", path)
	}
	err = d.pullAndImportFromECR(matches[1], matches[2])
	if err != nil {
		return nil, fmt.Errorf("failed to pull from ECR: %v", err)
	}
	return d.StorageDriver.GetContent(ctx, path)
}

func (m *ecrFetcher) pullAndImportFromECR(repo, tag string) error {
	fullImage := fmt.Sprintf("%s/%s:%s", m.remote, repo, tag)
	fmt.Print("Pulling image from ECR: ", fullImage)
	login := exec.Command("aws", "ecr", "get-login-password", "--region", "us-west-2")

	// aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 474353481944.dkr.ecr.us-east-1.amazonaws.com
	// Pull from ECR
	pull := exec.Command("docker", "pull", fullImage)
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		return err
	}
	logrus.Infof("Pulled image from ECR: %s", fullImage)
	// Tag and push to local registry
	localTag := fmt.Sprintf("%s/%s:%s", m.local, repo, tag)
	tagImage := exec.Command("docker", "tag", fullImage, localTag)
	tagImage.Stdout = os.Stdout
	tagImage.Stderr = os.Stderr
	if err := tagImage.Run(); err != nil {
		return err
	}
	logrus.Infof("Tagged image: %s -> %s", fullImage, localTag)
	push := exec.Command("docker", "push", localTag)
	push.Stdout = os.Stdout
	push.Stderr = os.Stderr
	if err := push.Run(); err != nil {
		return err
	}
	logrus.Infof("Pushed image to local registry: %s", localTag)

	return nil
}
