package websearch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

type Service struct {
	config Config
	client *http.Client
}

func New(config Config) (*Service, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	config.Providers = append([]ProviderID{}, config.Providers...)
	client, err := newSafeHTTPClient(config)
	if err != nil {
		return nil, err
	}
	config.HTTPClient = nil
	config.Resolver = nil
	return &Service{config: config, client: client}, nil
}

func (service *Service) Limits() AcquisitionLimits {
	if service == nil {
		return AcquisitionLimits{}
	}
	return AcquisitionLimits{MaxDocuments: service.config.MaxDocuments}
}

func (service *Service) Discover(ctx context.Context, request QueryRequest) (CandidateReport, error) {
	if err := service.validateBoundary(ctx); err != nil {
		return CandidateReport{}, err
	}
	query, err := validateQuery(request)
	if err != nil {
		return CandidateReport{}, err
	}
	report := CandidateReport{
		Query:       query,
		Candidates:  make([]Candidate, 0, service.config.MaxCandidates),
		Diagnostics: make([]ProviderDiagnostic, 0, len(service.config.Providers)),
	}
	indices := make(map[string]int, service.config.MaxCandidates)
	for _, providerID := range service.config.Providers {
		if err := ctx.Err(); err != nil {
			return CandidateReport{}, err
		}
		definition, ok := providerDefinitionFor(providerID)
		if !ok {
			return report, fmt.Errorf("%w: provider %q was not validated", ErrInvalidConfig, providerID)
		}
		searchURL := definition.searchURL(query)
		diagnostic := ProviderDiagnostic{Provider: providerID, SearchURL: searchURL}
		body, fetchErr := service.get(ctx, searchURL)
		if fetchErr != nil {
			if errors.Is(fetchErr, ErrUnsafeURL) || errors.Is(fetchErr, ErrInvalidFetchedText) {
				return service.cloneCandidateReport(report, fetchErr)
			}
			diagnostic.Outcome = DiscoveryFailed
			diagnostic.Failure = truncateUTF8(fetchErr.Error(), maxDiagnosticFailureBytes)
			report.Diagnostics = append(report.Diagnostics, diagnostic)
			if err := ctx.Err(); err != nil {
				return CandidateReport{}, err
			}
			continue
		}
		parsed, parseErr := parseProviderCandidates(
			definition, searchURL, body, service.config.MaxCandidatesPerProvider,
		)
		if parseErr != nil {
			return CandidateReport{}, parseErr
		}
		diagnostic.CandidateCount = len(parsed)
		if len(parsed) == 0 {
			diagnostic.Outcome = DiscoveryEmpty
			report.Diagnostics = append(report.Diagnostics, diagnostic)
			continue
		}
		diagnostic.Outcome = DiscoverySucceeded
		for rank, candidate := range parsed {
			if err := validateFetchedString("candidate title", candidate.title); err != nil {
				return service.cloneCandidateReport(report, err)
			}
			if err := validateFetchedString("candidate snippet", candidate.snippet); err != nil {
				return service.cloneCandidateReport(report, err)
			}
			source := CandidateSource{Provider: providerID, SearchURL: searchURL, Rank: rank + 1}
			if index, duplicate := indices[candidate.url]; duplicate {
				report.Candidates[index].Sources = append(report.Candidates[index].Sources, source)
				continue
			}
			if len(report.Candidates) == service.config.MaxCandidates {
				continue
			}
			indices[candidate.url] = len(report.Candidates)
			report.Candidates = append(report.Candidates, Candidate{
				ID: candidateID(candidate.url), URL: candidate.url,
				Title: candidate.title, Snippet: candidate.snippet,
				Sources: []CandidateSource{source},
			})
		}
		report.Diagnostics = append(report.Diagnostics, diagnostic)
	}
	if len(report.Candidates) == 0 {
		return service.cloneCandidateReport(report, fmt.Errorf("%w for query %q", ErrNoCandidates, query))
	}
	return service.cloneCandidateReport(report, nil)
}

func (service *Service) get(ctx context.Context, rawURL string) (string, error) {
	return service.getWithClient(ctx, rawURL, service.client)
}

func (service *Service) getDocument(ctx context.Context, rawURL string) (string, error) {
	if err := service.validateBoundary(ctx); err != nil {
		return "", err
	}
	client := *service.client
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		source := rawURL
		if len(via) > 0 && via[len(via)-1].URL != nil {
			source = via[len(via)-1].URL.String()
		}
		return fmt.Errorf("%w: %s redirects to %s", ErrDocumentRedirect, source, request.URL.String())
	}
	return service.getWithClient(ctx, rawURL, &client)
}

func (service *Service) getWithClient(ctx context.Context, rawURL string, client *http.Client) (string, error) {
	if err := service.validateBoundary(ctx); err != nil {
		return "", err
	}
	if client == nil {
		return "", fmt.Errorf("%w: HTTP client is nil", ErrInvalidConfig)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build HTTP request: %w", err)
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; omnidex/1.0)")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("execute HTTP request: %w", err)
	}
	defer response.Body.Close()
	reader := io.LimitReader(response.Body, service.config.MaxResponseBytes+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read HTTP response: %w", err)
	}
	if int64(len(body)) > service.config.MaxResponseBytes {
		return "", fmt.Errorf("HTTP response exceeds %d bytes", service.config.MaxResponseBytes)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateFetchedBytes("HTTP response body", body); err != nil {
		return "", err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("HTTP status %d: %s", response.StatusCode, truncateUTF8Bytes(body, 240))
	}
	return string(body), nil
}

func truncateUTF8Bytes(value []byte, limit int) string {
	if len(value) <= limit {
		return string(value)
	}
	bounded := value[:limit]
	for len(bounded) > 0 && !utf8.Valid(bounded) {
		bounded = bounded[:len(bounded)-1]
	}
	return strings.TrimSpace(string(bounded))
}

func (service *Service) cloneCandidateReport(report CandidateReport, runErr error) (CandidateReport, error) {
	if err := validateCandidateReportBounds(
		report, service.config.MaxCandidates, len(service.config.Providers),
	); err != nil {
		return CandidateReport{}, err
	}
	copy := report
	copy.Diagnostics = append([]ProviderDiagnostic{}, report.Diagnostics...)
	copy.Candidates = make([]Candidate, len(report.Candidates))
	for index, candidate := range report.Candidates {
		copy.Candidates[index] = candidate
		copy.Candidates[index].Sources = append([]CandidateSource{}, candidate.Sources...)
	}
	return copy, runErr
}

func (service *Service) validateBoundary(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if service == nil || service.client == nil {
		return fmt.Errorf("%w: service is nil or uninitialized", ErrInvalidConfig)
	}
	return nil
}
