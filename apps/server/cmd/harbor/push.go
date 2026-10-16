package main

import (
	"bytes"
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
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("name", *name)
	_ = w.WriteField("kind", *kind)
	_ = w.WriteField("source", *source)
	_ = w.WriteField("note", *note)
	_ = w.WriteField("tags", *tags)
	part, err := w.CreateFormFile("file", filepath.Base(*file))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(*endpoint, "/")+"/api/v1/push", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+*token)
	req.Header.Set("Content-Type", w.FormDataContentType())
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
