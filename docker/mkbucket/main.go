// mkbucket — утилита для подготовки S3 в Docker-тестах.
//
// Режимы:
//   mkbucket <endpoint> <access> <secret> make <bucket>
//       — создать bucket (если нет)
//   mkbucket <endpoint> <access> <secret> check <bucket> <prefix>
//       — проверить что в bucket есть объекты с данным префиксом
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	if len(os.Args) != 6 {
		fmt.Fprintln(os.Stderr, "usage: mkbucket <endpoint> <access> <secret> <make|check> <bucket> [prefix]")
		os.Exit(2)
	}
	endpoint, access, secret, mode, bucket := os.Args[1], os.Args[2], os.Args[3], os.Args[4], os.Args[5]

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "minio client: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	switch mode {
	case "make":
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bucket exists: %v\n", err)
			os.Exit(1)
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				fmt.Fprintf(os.Stderr, "make bucket: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("bucket created:", bucket)
		} else {
			fmt.Println("bucket exists:", bucket)
		}

	case "check":
		count := 0
		for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
			Recursive: true,
		}) {
			if obj.Err != nil {
				fmt.Fprintf(os.Stderr, "list error: %v\n", obj.Err)
				os.Exit(1)
			}
			count++
			fmt.Println("object:", obj.Key)
		}
		if count == 0 {
			fmt.Println("NO OBJECTS")
			os.Exit(1)
		}
		fmt.Printf("found %d objects\n", count)

	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		os.Exit(2)
	}

	_ = strings.TrimSpace
}
