package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func push(args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	endpoint := fs.String("endpoint", env("HARBOR_ENDPOINT", "http://127.0.0.1:8787"), "Harbor base URL")
	token := fs.String("token", os.Getenv("HARBOR_TOKEN"), "project API key")
	file := fs.String("file", "", "file to upload")
	name := fs.String("name", "", "backup name")
	kind := fs.String("kind", "archive", "archive, database, files, or other")
	source := fs.String("source", "", "application that produced the backup")
	note := fs.String("note", "", "optional note")
	tags := fs.String("tags", "", "comma-separated tags")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token == "" || *file == "" {
		return fmt.Errorf("push requires -file and -token")
	}
	f, err := os.Open(*file)
	if err != nil {
		return err
	}

	body, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	go func() {
		var copyErr error
		defer func() {
			f.Close()
			if copyErr != nil {
				writer.CloseWithError(copyErr)
				return
			}
			writer.CloseWithError(form.Close())
		}()
		if copyErr = form.WriteField("name", *name); copyErr != nil {
			return
		}
		if copyErr = form.WriteField("kind", *kind); copyErr != nil {
			return
		}
		if copyErr = form.WriteField("source", *source); copyErr != nil {
			return
		}
		if copyErr = form.WriteField("note", *note); copyErr != nil {
			return
		}
		if copyErr = form.WriteField("tags", *tags); copyErr != nil {
			return
		}
		part, err := form.CreateFormFile("file", filepath.Base(*file))
		if err != nil {
			copyErr = err
			return
		}
		_, copyErr = io.Copy(part, f)
	}()

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(*endpoint, "/")+"/api/v1/push", body)
	if err != nil {
		body.Close()
		return err
	}
	req.Header.Set("Authorization", "Bearer "+*token)
	req.Header.Set("Content-Type", form.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("push failed: %s", strings.TrimSpace(string(payload)))
	}
	var out struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Checksum string `json:"checksum_sha256"`
		Size     int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		fmt.Println(string(payload))
		return nil
	}
	fmt.Printf("stored %s (%d bytes) checksum %s id %s\n", out.Name, out.Size, out.Checksum, out.ID)
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
