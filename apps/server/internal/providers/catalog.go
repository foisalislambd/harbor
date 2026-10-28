// Package providers is the list of object-storage services Harbor can copy
// backups to. Each entry uses the endpoint and signing region that service
// documents for its S3-compatible API.
package providers

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"harbor/internal/remote/r2"
	"harbor/internal/store"
)

const (
	free10GB int64 = 10_000_000_000
)

// Provider is one storage service shown in the panel.
type Provider struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Group               string   `json:"group"`
	Summary             string   `json:"summary"`
	Steps               []string `json:"steps"`
	FreeBytes           int64    `json:"free_bytes"`
	FreeNote            string   `json:"free_note"`
	NeedsAccountID      bool     `json:"account_id"`
	NeedsJurisdiction   bool     `json:"jurisdiction"`
	NeedsRegion         bool     `json:"region"`
	RegionPlaceholder   string   `json:"region_placeholder"`
	NeedsEndpoint       bool     `json:"endpoint"`
	EndpointPlaceholder string   `json:"endpoint_placeholder"`
	AllowHTTP           bool     `json:"allow_http"`
	endpointTemplate    string
	fixedEndpoint       string
	sigRegion           string
	storageClass        string
	versioned           bool
	virtualHost         bool
}

// All returns the services a person can connect, common ones first.
func All() []Provider {
	out := make([]Provider, len(catalog))
	copy(out, catalog)
	return out
}

// ByID finds a service.
func ByID(id string) (Provider, bool) {
	for _, provider := range catalog {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}

// Label is the name shown when a copy cannot be stored.
func Label(id string) string {
	if provider, ok := ByID(id); ok {
		return provider.Name
	}
	return "This destination"
}

// FreeBytes is the always-free storage Harbor will not pass unless the
// destination allows it. Zero means the service has no permanent free allowance.
func FreeBytes(id string) int64 {
	provider, ok := ByID(id)
	if !ok {
		return 0
	}
	return provider.FreeBytes
}

// Prepare checks the fields this service needs and normalizes them.
func Prepare(d store.Destination) (store.Destination, error) {
	provider, ok := ByID(d.Provider)
	if !ok {
		return store.Destination{}, errors.New("choose a storage service")
	}
	d.Region = strings.ToLower(strings.TrimSpace(d.Region))
	d.Endpoint = strings.TrimSpace(d.Endpoint)
	d.Jurisdiction = strings.ToLower(strings.TrimSpace(d.Jurisdiction))
	if d.Jurisdiction == "default" {
		d.Jurisdiction = ""
	}
	if !provider.NeedsJurisdiction {
		d.Jurisdiction = ""
	}
	if !provider.NeedsRegion {
		d.Region = ""
	}
	if !provider.NeedsEndpoint {
		d.Endpoint = ""
	}
	if !provider.NeedsAccountID {
		d.AccountID = ""
	}
	if provider.FreeBytes == 0 {
		d.AllowOverFree = false
	}
	if _, _, err := endpointFor(provider, d); err != nil {
		return store.Destination{}, err
	}
	return d, nil
}

// Open builds the client that copies objects for this destination.
func Open(d store.Destination) (*r2.Client, error) {
	provider, ok := ByID(d.Provider)
	if !ok {
		return nil, errors.New("choose a storage service")
	}
	endpoint, region, err := endpointFor(provider, d)
	if err != nil {
		return nil, err
	}
	return &r2.Client{
		Endpoint:     endpoint,
		Bucket:       d.Bucket,
		AccessKey:    d.AccessKeyID,
		Secret:       d.Secret,
		Region:       region,
		StorageClass: provider.storageClass,
		Versioned:    provider.versioned,
		VirtualHost:  provider.virtualHost,
	}, nil
}

func endpointFor(provider Provider, d store.Destination) (string, string, error) {
	if provider.ID == store.ProviderCloudflareR2 {
		endpoint, err := r2.EndpointURL(d.AccountID, d.Jurisdiction)
		return endpoint, "auto", err
	}
	if provider.NeedsRegion && !regionPattern(d.Region) {
		return "", "", fmt.Errorf("enter the %s region, for example %s", provider.Name, provider.RegionPlaceholder)
	}
	endpoint := provider.fixedEndpoint
	if provider.endpointTemplate != "" {
		endpoint = fmt.Sprintf(provider.endpointTemplate, d.Region)
	}
	if provider.NeedsEndpoint {
		endpoint = d.Endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("enter the endpoint exactly as the service shows it")
	}
	if parsed.Scheme == "http" && !provider.AllowHTTP {
		return "", "", errors.New("this service only accepts an https endpoint")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", "", errors.New("the endpoint must start with https://")
	}
	region := provider.sigRegion
	if region == "region" {
		region = d.Region
	}
	if region == "" {
		region = "us-east-1"
	}
	return strings.TrimRight(endpoint, "/"), region, nil
}

func regionPattern(region string) bool {
	if region == "" || len(region) > 64 {
		return false
	}
	for i, r := range region {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && i > 0:
		default:
			return false
		}
	}
	return true
}

func withVirtual(provider Provider) Provider {
	provider.virtualHost = true
	return provider
}

func regionService(id, name, group, summary, placeholder, template string, steps []string, free int64, note, class string, versioned bool) Provider {
	return Provider{
		ID: id, Name: name, Group: group, Summary: summary, Steps: steps,
		FreeBytes: free, FreeNote: note, NeedsRegion: true, RegionPlaceholder: placeholder,
		endpointTemplate: template, sigRegion: "region", storageClass: class, versioned: versioned,
	}
}

var catalog = []Provider{
	{
		ID: store.ProviderCloudflareR2, Name: "Cloudflare R2", Group: "Common",
		Summary: "10 GB of Standard storage free each month, and egress is free.",
		Steps: []string{
			"In the Cloudflare dashboard, open R2 and create a bucket. Leave it on Standard storage.",
			"On R2 Overview, choose Manage API tokens, then Create Account API token.",
			"Give it Object Read & Write for that bucket only.",
			"Copy the Access Key ID, the Secret Access Key, and the account ID from the S3 endpoint.",
		},
		FreeBytes: free10GB, FreeNote: "Harbor stops at 10 GB unless you allow more. Infrequent Access is not free.",
		NeedsAccountID: true, NeedsJurisdiction: true, storageClass: "STANDARD",
	},
	regionService("backblaze_b2", "Backblaze B2", "Common",
		"The first 10 GB stored on the account is free.",
		"us-west-004", "https://s3.%s.backblazeb2.com",
		[]string{
			"Create a B2 bucket. The S3 endpoint looks like s3.us-west-004.backblazeb2.com.",
			"The region is the middle of that host, for example us-west-004.",
			"Create an application key that can read and write this bucket. The master key does not work with the S3 API.",
			"Use the keyID as the access key and the applicationKey as the secret.",
		}, free10GB, "Harbor stops at 10 GB per access key unless you allow more. Use one key for the account. Uploads are free. Downloads past three times the stored data are billed.", "", true),
	regionService("amazon_s3", "Amazon S3", "Common",
		"The usual paid object storage. New AWS accounts can have a short 5 GB trial.",
		"us-east-1", "https://s3.%s.amazonaws.com",
		[]string{
			"Create a bucket and note its region, such as us-east-1 or eu-west-1.",
			"Create an IAM user with access to that bucket only, then create an access key.",
			"Paste the access key ID and the secret access key.",
		}, 0, "There is no permanent free allowance. A new account's 12-month trial is separate from this cap.", "STANDARD", false),
	regionService("wasabi", "Wasabi", "Common",
		"Hot storage with no egress fee. There is no permanent free tier.",
		"us-east-1", "https://s3.%s.wasabisys.com",
		[]string{
			"Create a bucket and copy its region from the endpoint, such as s3.us-east-1.wasabisys.com.",
			"Create an access key for that bucket.",
		}, 0, "Wasabi bills for stored data. A trial, if you have one, is on their account, not a cap Harbor enforces.", "STANDARD", false),
	{
		ID: "google_cloud_storage", Name: "Google Cloud Storage", Group: "Common",
		Summary: "Uses an HMAC key, not a service-account JSON file.",
		Steps: []string{
			"Create a bucket.",
			"In Cloud Storage, open Settings, then Interoperability, and create an HMAC key for a service account.",
			"Paste that access key and secret. A JSON key file will not work here.",
		},
		FreeNote:      "Google's free trial is on the billing account. Harbor does not apply a storage cap.",
		fixedEndpoint: "https://storage.googleapis.com",
		sigRegion:     "auto",
		storageClass:  "STANDARD",
	},
	regionService("digitalocean_spaces", "DigitalOcean Spaces", "Common",
		"A Space is DigitalOcean's S3 bucket.",
		"nyc3", "https://%s.digitaloceanspaces.com",
		[]string{
			"Create a Space and note its region: nyc3, sfo3, ams3, sgp1, fra1, or syd1.",
			"Under API, create a Spaces access key. It is separate from a DigitalOcean API token.",
		}, 0, "A Space plan includes a monthly allowance. Harbor does not cap it.", "", false),
	regionService("hetzner", "Hetzner Object Storage", "More services",
		"S3 storage in Hetzner locations.",
		"fsn1", "https://%s.your-objectstorage.com",
		[]string{
			"Create an Object Storage bucket. Locations are fsn1, nbg1, and hel1.",
			"Under Security, then Object Storage, create credentials.",
		}, 0, "", "", false),
	regionService("scaleway", "Scaleway Object Storage", "More services",
		"European S3 storage.",
		"fr-par", "https://s3.%s.scw.cloud",
		[]string{
			"Create a bucket in fr-par, nl-ams, or pl-waw.",
			"Create an API key that can use Object Storage.",
		}, 0, "", "", false),
	regionService("linode", "Akamai Linode Object Storage", "More services",
		"Linode's S3 storage, now part of Akamai.",
		"us-east-1", "https://%s.linodeobjects.com",
		[]string{
			"Create a bucket and copy the region in its host, such as us-east-1.linodeobjects.com.",
			"Create an Object Storage access key.",
		}, 0, "", "", false),
	regionService("vultr", "Vultr Object Storage", "More services",
		"S3 storage on Vultr.",
		"ewr1", "https://%s.vultrobjects.com",
		[]string{
			"Subscribe to Object Storage and create a bucket.",
			"The region is the start of the host, such as ewr1 in ewr1.vultrobjects.com.",
			"Copy the access key and secret from the bucket's S3 credentials.",
		}, 0, "", "", false),
	regionService("ovh", "OVHcloud Object Storage", "More services",
		"OVH's S3-compatible storage.",
		"gra", "https://s3.%s.io.cloud.ovh.net",
		[]string{
			"Create an Object Storage bucket and an S3 user.",
			"The region is short, such as gra, sbg, bhs, de, uk, or waw.",
		}, 0, "", "", false),
	regionService("exoscale", "Exoscale SOS", "More services",
		"Exoscale Simple Object Storage.",
		"de-fra-1", "https://sos-%s.exo.io",
		[]string{
			"Create an SOS bucket. The zone looks like de-fra-1 or ch-gva-2.",
			"Create an IAM key that can use that bucket.",
		}, 0, "", "", false),
	{
		ID: "storj", Name: "Storj", Group: "More services",
		Summary: "Decentralized storage with an S3 gateway.",
		Steps: []string{
			"Create a bucket in the Storj console.",
			"Create S3 credentials for that bucket.",
			"The gateway is gateway.storjshare.io. You do not pick a region.",
		},
		FreeNote:      "Storj's free storage, if the account still has it, is managed by Storj.",
		fixedEndpoint: "https://gateway.storjshare.io",
		sigRegion:     "us-east-1",
	},
	{
		ID: "tigris", Name: "Tigris", Group: "More services",
		Summary: "S3 storage with a single global endpoint.",
		Steps: []string{
			"Create a bucket in the Tigris console.",
			"Create an access key and use the endpoint t3.storage.dev.",
		},
		fixedEndpoint: "https://t3.storage.dev",
		sigRegion:     "auto",
	},
	{
		ID: "filebase", Name: "Filebase", Group: "More services",
		Summary: "S3 gateway in front of decentralized storage.",
		Steps: []string{
			"Create a bucket.",
			"Copy the Access Key and Secret from the Filebase console.",
		},
		fixedEndpoint: "https://s3.filebase.com",
		sigRegion:     "us-east-1",
	},
	{
		ID: "idrive_e2", Name: "IDrive e2", Group: "More services",
		Summary: "Copy the endpoint from the e2 console. It is different for each region.",
		Steps: []string{
			"Create an e2 bucket.",
			"Copy the S3 endpoint shown for that bucket. It looks like https://s3.region.idrivee2.com.",
			"Create an access key and paste the region shown next to the endpoint.",
		},
		NeedsRegion: true, RegionPlaceholder: "us-east",
		NeedsEndpoint: true, EndpointPlaceholder: "https://s3.region.idrivee2.com",
		sigRegion: "region",
	},
	{
		ID: "oracle", Name: "Oracle Cloud Object Storage", Group: "More services",
		Summary: "Uses a Customer Secret Key and the compatibility endpoint.",
		Steps: []string{
			"Create a bucket and a Customer Secret Key. An OCI API signing key will not work.",
			"The endpoint looks like https://NAMESPACE.compat.objectstorage.REGION.oraclecloud.com.",
			"The region is the one in that host, such as eu-frankfurt-1.",
		},
		NeedsRegion: true, RegionPlaceholder: "eu-frankfurt-1",
		NeedsEndpoint: true, EndpointPlaceholder: "https://namespace.compat.objectstorage.eu-frankfurt-1.oraclecloud.com",
		sigRegion: "region",
	},
	regionService("ibm", "IBM Cloud Object Storage", "More services",
		"HMAC credentials, not an IAM API key by itself.",
		"us-east", "https://s3.%s.cloud-object-storage.appdomain.cloud",
		[]string{
			"Create a bucket. The region is short, such as us-east, eu-de, or jp-tok.",
			"Create HMAC credentials for a service ID that can write to the bucket.",
		}, 0, "", "", false),
	withVirtual(regionService("alibaba", "Alibaba Cloud OSS", "More services",
		"S3 access uses the s3.oss region endpoint. The bucket name cannot contain a dot.",
		"cn-hangzhou", "https://s3.oss-%s.aliyuncs.com",
		[]string{
			"Create a bucket and copy its region ID, such as cn-hangzhou or eu-central-1.",
			"Create an AccessKey pair that can use that bucket.",
			"Buckets in mainland China may need a custom domain before the public endpoint accepts uploads.",
		}, 0, "", "", false)),
	withVirtual(regionService("tencent", "Tencent Cloud COS", "More services",
		"S3-compatible access. The bucket name includes the APPID.",
		"ap-guangzhou", "https://cos.%s.myqcloud.com",
		[]string{
			"Create a bucket and note its region, such as ap-guangzhou.",
			"The bucket name to enter is the full name, such as mybucket-1250000000.",
			"Create a SecretId and SecretKey that can read and write the bucket.",
		}, 0, "", "", false)),
	{
		ID: "contabo", Name: "Contabo Object Storage", Group: "More services",
		Summary: "S3 storage from Contabo. Requests are signed with the region name default.",
		Steps: []string{
			"Create Object Storage and copy the S3 base URL.",
			"The location is the first part of that host: eu2, usc1, or sin1.",
			"Copy the access key and secret from Security and Access.",
		},
		NeedsRegion: true, RegionPlaceholder: "eu2",
		endpointTemplate: "https://%s.contabostorage.com", sigRegion: "default",
	},
	{
		ID: "supabase", Name: "Supabase Storage", Group: "More services",
		Summary: "The S3 endpoint of a Supabase project.",
		Steps: []string{
			"Open Storage, then S3, in the Supabase project.",
			"Copy the endpoint. It looks like https://PROJECT.supabase.co/storage/v1/s3.",
			"Create an access key and paste it here. Do not include the bucket in the endpoint.",
		},
		NeedsEndpoint: true, EndpointPlaceholder: "https://project.supabase.co/storage/v1/s3",
		sigRegion: "us-east-1",
	},
	{
		ID: "mega", Name: "MEGA S4", Group: "More services",
		Summary: "MEGA's S3 object storage. Copy the endpoint from their console.",
		Steps: []string{
			"Create an S4 bucket.",
			"Copy the S3 endpoint and region shown for that bucket.",
			"Create S3 credentials and paste them here.",
		},
		NeedsRegion: true, RegionPlaceholder: "eu-central-1",
		NeedsEndpoint: true, EndpointPlaceholder: "https://s3.eu-central-1.s4.mega.io",
		sigRegion: "region",
	},
	{
		ID: "minio", Name: "MinIO", Group: "Self-hosted",
		Summary: "Any MinIO server you run.",
		Steps: []string{
			"Use the server address, such as http://127.0.0.1:9000 or https://minio.example.com.",
			"The region is usually us-east-1 unless you changed it.",
			"Paste the access key and secret for a user that can write the bucket.",
		},
		NeedsRegion: true, RegionPlaceholder: "us-east-1",
		NeedsEndpoint: true, EndpointPlaceholder: "http://127.0.0.1:9000",
		AllowHTTP: true, sigRegion: "region",
	},
	{
		ID: "ceph", Name: "Ceph RGW", Group: "Self-hosted",
		Summary: "A Ceph RADOS Gateway.",
		Steps: []string{
			"Use the gateway address, often port 7480.",
			"Paste the S3 access key and secret for the bucket.",
		},
		NeedsRegion: true, RegionPlaceholder: "default",
		NeedsEndpoint: true, EndpointPlaceholder: "http://127.0.0.1:7480",
		AllowHTTP: true, sigRegion: "region",
	},
	{
		ID: "garage", Name: "Garage", Group: "Self-hosted",
		Summary: "The Garage object store.",
		Steps: []string{
			"Use the S3 API address, often port 3900.",
			"The region is garage unless you configured another.",
			"Paste the key and secret created in Garage.",
		},
		NeedsRegion: true, RegionPlaceholder: "garage",
		NeedsEndpoint: true, EndpointPlaceholder: "http://127.0.0.1:3900",
		AllowHTTP: true, sigRegion: "region",
	},
	{
		ID: "seaweedfs", Name: "SeaweedFS", Group: "Self-hosted",
		Summary: "The SeaweedFS S3 port.",
		Steps: []string{
			"Use the S3 address, often port 8333.",
			"Paste the access key configured for that gateway.",
		},
		NeedsRegion: true, RegionPlaceholder: "us-east-1",
		NeedsEndpoint: true, EndpointPlaceholder: "http://127.0.0.1:8333",
		AllowHTTP: true, sigRegion: "region",
	},
	{
		ID: "custom", Name: "Other S3-compatible storage", Group: "Self-hosted",
		Summary: "Any other service that speaks the S3 API.",
		Steps: []string{
			"Copy the S3 endpoint from that service. Leave the bucket name out of the path.",
			"Enter the region the service uses for request signing. If it does not say, try us-east-1.",
			"Create a key that can read and write one bucket, then paste it here.",
		},
		NeedsRegion: true, RegionPlaceholder: "us-east-1",
		NeedsEndpoint: true, EndpointPlaceholder: "https://s3.example.com",
		AllowHTTP: true, sigRegion: "region",
	},
}
