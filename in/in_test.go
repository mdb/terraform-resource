package main_test

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"

	"github.com/ljfranklin/terraform-resource/models"
	"github.com/ljfranklin/terraform-resource/storage"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("In", func() {

	var (
		s3              storage.Storage
		inReq           models.InRequest
		pathToS3Fixture string
		tmpDir          string
	)

	BeforeEach(func() {
		accessKey := os.Getenv("AWS_ACCESS_KEY")
		Expect(accessKey).ToNot(BeEmpty(), "AWS_ACCESS_KEY must be set")

		secretKey := os.Getenv("AWS_SECRET_KEY")
		Expect(secretKey).ToNot(BeEmpty(), "AWS_SECRET_KEY must be set")

		bucket := os.Getenv("AWS_BUCKET")
		Expect(bucket).ToNot(BeEmpty(), "AWS_BUCKET must be set")

		bucketPath := os.Getenv("AWS_BUCKET_PATH") // optional

		region := os.Getenv("AWS_REGION") // optional

		s3 = storage.NewS3(accessKey, secretKey, region, bucket)

		pathToS3Fixture = path.Join(bucketPath, randomString("s3-test-fixture"))

		fixture, err := os.Open(getFileLocation("fixtures/s3/terraform.tfstate"))
		Expect(err).ToNot(HaveOccurred())
		defer fixture.Close()

		err = s3.Upload(pathToS3Fixture, fixture)
		Expect(err).ToNot(HaveOccurred())

		inReq = models.InRequest{
			Source: models.Source{
				Bucket:          bucket,
				Key:             pathToS3Fixture,
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
			},
		}

		tmpDir, err = ioutil.TempDir("", "terraform-resource-in-test")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = s3.Delete(pathToS3Fixture) // ignore error on cleanup
		_ = os.RemoveAll(tmpDir)
	})

	It("fetches state file from S3", func() {
		expectedOutputPath := path.Join(tmpDir, path.Base(pathToS3Fixture))
		Expect(expectedOutputPath).ToNot(BeAnExistingFile())

		command := exec.Command(pathToInBinary, tmpDir)

		stdin, err := command.StdinPipe()
		Expect(err).ToNot(HaveOccurred())

		session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		err = json.NewEncoder(stdin).Encode(inReq)
		Expect(err).ToNot(HaveOccurred())
		stdin.Close()

		Eventually(session).Should(gexec.Exit(0))

		actualOutput := models.InResponse{}
		err = json.Unmarshal(session.Out.Contents(), &actualOutput)
		Expect(err).ToNot(HaveOccurred())

		// does version match format "2006-01-02T15:04:05Z"
		_, err = time.Parse(time.RFC3339, actualOutput.Version.Version)
		Expect(err).ToNot(HaveOccurred())

		Expect(expectedOutputPath).To(BeAnExistingFile())
		outputFile, err := os.Open(expectedOutputPath)
		Expect(err).ToNot(HaveOccurred())
		defer outputFile.Close()

		outputContents := map[string]interface{}{}
		err = json.NewDecoder(outputFile).Decode(&outputContents)
		Expect(err).ToNot(HaveOccurred())

		Expect(outputContents["vpc_id"]).ToNot(BeNil())
	})
})

func randomString(prefix string) string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	Expect(err).ToNot(HaveOccurred())
	return fmt.Sprintf("%s-%x", prefix, b)
}

func getFileLocation(relativePath string) string {
	_, filename, _, _ := runtime.Caller(1)
	return path.Join(path.Dir(filename), "..", relativePath)
}
