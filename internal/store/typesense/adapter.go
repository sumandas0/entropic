package typesense

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/sumandas0/entropic/internal/integration"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/observability"
	"github.com/sumandas0/entropic/internal/store"
	"github.com/sumandas0/entropic/pkg/utils"
	"github.com/typesense/typesense-go/typesense"
	"github.com/typesense/typesense-go/typesense/api"
	"github.com/typesense/typesense-go/typesense/api/pointer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type TypesenseStore struct {
	client     *typesense.Client
	obsManager *integration.ObservabilityManager
	logger     zerolog.Logger
	tracer     trace.Tracer
	tracing    *observability.TracingManager
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

func NewTypesenseStore(serverURL, apiKey string) (*TypesenseStore, error) {
	client := typesense.NewClient(
		typesense.WithServer(serverURL),
		typesense.WithAPIKey(apiKey),
		typesense.WithConnectionTimeout(5*time.Second),
	)

	return &TypesenseStore{
		client: client,
	}, nil
}

func (s *TypesenseStore) SetObservability(obsManager *integration.ObservabilityManager) {
	if obsManager != nil {
		s.obsManager = obsManager
		s.logger = obsManager.GetLogging().GetZerologLogger()
		s.tracer = obsManager.GetTracing().GetTracer()
		s.tracing = obsManager.GetTracing()
	}
}

func (s *TypesenseStore) IndexEntity(ctx context.Context, entity *models.Entity) error {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.index_entity")
		defer span.End()
		span.SetAttributes(
			attribute.String("entity.type", entity.EntityType),
			attribute.String("entity.id", entity.ID.String()),
		)
	}

	collectionName := s.getCollectionName(entity.EntityType)

	document := s.flattenEntity(entity)

	_, err := s.client.Collection(collectionName).Documents().Upsert(ctx, document)
	if err != nil {
		// If collection doesn't exist (404), create it with a basic schema
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
			// Create a basic schema for the collection
			basicSchema := &models.EntitySchema{
				EntityType: entity.EntityType,
				Properties: make(models.PropertySchema),
			}

			// Infer properties from the entity
			for key, value := range entity.Properties {
				propDef := models.PropertyDefinition{
					Type:     s.inferPropertyType(value),
					Required: false,
				}
				
				// For arrays, try to infer the element type
				if propDef.Type == "array" {
					switch v := value.(type) {
					case []string:
						propDef.ElementType = "string"
					case []int, []int32, []int64:
						propDef.ElementType = "number"
					case []any:
						if len(v) > 0 {
							// Infer from first element
							propDef.ElementType = s.inferPropertyType(v[0])
						} else {
							propDef.ElementType = "string" // Default to string
						}
					}
				}
				
				basicSchema.Properties[key] = propDef
			}

			// Try to create the collection
			if createErr := s.CreateCollection(ctx, entity.EntityType, basicSchema); createErr != nil {
				if s.tracing != nil {
					s.tracing.SetSpanError(span, createErr)
				}
				return fmt.Errorf("failed to create collection: %w", createErr)
			}

			// Retry the upsert
			_, err = s.client.Collection(collectionName).Documents().Upsert(ctx, document)
			if err != nil {
				if s.tracing != nil {
					s.tracing.SetSpanError(span, err)
				}
				return fmt.Errorf("failed to index entity after creating collection: %w", err)
			}
			return nil
		}

		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to index entity: %w", err)
	}

	return nil
}

func (s *TypesenseStore) UpdateEntityIndex(ctx context.Context, entity *models.Entity) error {
	// UpdateEntityIndex uses IndexEntity with upsert
	return s.IndexEntity(ctx, entity)
}

func (s *TypesenseStore) DeleteEntityIndex(ctx context.Context, entityType string, id uuid.UUID) error {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.delete_entity_index")
		defer span.End()
		span.SetAttributes(
			attribute.String("entity.type", entityType),
			attribute.String("entity.id", id.String()),
		)
	}

	collectionName := s.getCollectionName(entityType)

	_, err := s.client.Collection(collectionName).Document(id.String()).Delete(ctx)
	if err != nil {
		// Not found is not an error for deletion
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "404") {
			return nil
		}
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to delete entity from index: %w", err)
	}

	return nil
}

func (s *TypesenseStore) Search(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error) {
	var span trace.Span
	if s.tracing != nil {
		ctx, span = s.tracing.StartSearchOperation(ctx, "text", query.Query, query.EntityTypes)
		defer span.End()
	}

	if len(query.EntityTypes) == 0 {
		err := utils.NewAppError(utils.CodeInvalidInput, "at least one entity type is required", nil)
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return nil, err
	}
	
	if query.Limit <= 0 || query.Limit > 1000 {
		err := utils.NewAppError(utils.CodeInvalidInput, "limit must be between 1 and 1000", nil)
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return nil, err
	}

	if len(query.EntityTypes) > 1 {
		return s.multiSearch(ctx, query)
	}

	collectionName := s.getCollectionName(query.EntityTypes[0])

	searchParams := &api.SearchCollectionParams{
		Q:       query.Query,
		QueryBy: "*",
		Page:    pointer.Int(s.calculatePage(query.Offset, query.Limit)),
		PerPage: pointer.Int(query.Limit),
	}

	if len(query.Filters) > 0 {
		filterStr := s.buildFilterString(query.Filters)
		if filterStr != "" {
			searchParams.FilterBy = pointer.String(filterStr)
		}
	}

	if len(query.Sort) > 0 {
		sortStr := s.buildSortString(query.Sort)
		searchParams.SortBy = pointer.String(sortStr)
	}

	if len(query.Facets) > 0 {
		searchParams.FacetBy = pointer.String(strings.Join(query.Facets, ","))
	}

	startTime := time.Now()
	result, err := s.client.Collection(collectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return s.convertSearchResult(result, query, time.Since(startTime)), nil
}

func (s *TypesenseStore) VectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error) {
	var span trace.Span
	if s.tracing != nil {
		ctx, span = s.tracing.StartSearchOperation(ctx, "vector", query.VectorField, query.EntityTypes)
		defer span.End()
	}

	if len(query.EntityTypes) == 0 {
		err := utils.NewAppError(utils.CodeInvalidInput, "at least one entity type is required", nil)
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return nil, err
	}
	
	if query.TopK <= 0 || query.TopK > 1000 {
		err := utils.NewAppError(utils.CodeInvalidInput, "top_k must be between 1 and 1000", nil)
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return nil, err
	}

	// Always use multi-search for vector queries to avoid query length limits
	return s.multiVectorSearch(ctx, query)
}

func (s *TypesenseStore) CreateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.create_collection")
		defer span.End()
		span.SetAttributes(attribute.String("entity.type", entityType))
	}

	collectionName := s.getCollectionName(entityType)

	fields := s.buildFieldsFromSchema(schema)

	fields = append(fields,
		api.Field{
			Name: "id",
			Type: "string",
		},
		api.Field{
			Name: "urn",
			Type: "string",
		},
		api.Field{
			Name:  "created_at",
			Type:  "int64",
			Facet: boolPtr(true),
		},
		api.Field{
			Name:  "updated_at",
			Type:  "int64",
			Facet: boolPtr(true),
		},
	)

	collectionSchema := &api.CollectionSchema{
		Name:                collectionName,
		Fields:              fields,
		DefaultSortingField: stringPtr("updated_at"),
		EnableNestedFields:  boolPtr(true),
	}

	_, err := s.client.Collections().Create(ctx, collectionSchema)
	if err != nil {
		// If collection already exists, try to update it
		if strings.Contains(err.Error(), "already exists") {
			return s.UpdateCollection(ctx, entityType, schema)
		}
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

func (s *TypesenseStore) UpdateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.update_collection")
		defer span.End()
		span.SetAttributes(attribute.String("entity.type", entityType))
	}

	collectionName := s.getCollectionName(entityType)

	existingCollection, err := s.client.Collection(collectionName).Retrieve(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// If collection doesn't exist, create it
			return s.CreateCollection(ctx, entityType, schema)
		}
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to retrieve collection: %w", err)
	}

	newFields := s.buildFieldsFromSchema(schema)

	existingFieldMap := make(map[string]bool)
	for _, field := range existingCollection.Fields {
		existingFieldMap[field.Name] = true
	}

	for _, field := range newFields {
		if !existingFieldMap[field.Name] {
			updateSchema := &api.CollectionUpdateSchema{
				Fields: []api.Field{field},
			}
			_, err := s.client.Collection(collectionName).Update(ctx, updateSchema)
			if err != nil {
				if s.tracing != nil {
					s.tracing.SetSpanError(span, err)
				}
				return fmt.Errorf("failed to add field %s: %w", field.Name, err)
			}
		}
	}

	return nil
}

func (s *TypesenseStore) DeleteCollection(ctx context.Context, entityType string) error {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.delete_collection")
		defer span.End()
		span.SetAttributes(attribute.String("entity.type", entityType))
	}

	collectionName := s.getCollectionName(entityType)

	_, err := s.client.Collection(collectionName).Delete(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	return nil
}

func (s *TypesenseStore) Ping(ctx context.Context) error {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.ping")
		defer span.End()
	}

	_, err := s.client.Health(ctx, 5*time.Second)
	if err != nil && s.tracing != nil {
		s.tracing.SetSpanError(span, err)
	}
	return err
}

func (s *TypesenseStore) Close() error {
	return nil
}

func (s *TypesenseStore) getCollectionName(entityType string) string {

	name := strings.ToLower(entityType)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return "entity_" + name
}

func (s *TypesenseStore) flattenEntity(entity *models.Entity) map[string]any {
	doc := make(map[string]any)

	doc["id"] = entity.ID.String()
	doc["urn"] = entity.URN
	doc["entity_type"] = entity.EntityType
	doc["created_at"] = entity.CreatedAt.Unix()
	doc["updated_at"] = entity.UpdatedAt.Unix()

	for key, value := range entity.Properties {
		doc[key] = value
	}

	return doc
}

func (s *TypesenseStore) buildFieldsFromSchema(schema *models.EntitySchema) []api.Field {
	var fields []api.Field

	for propName, propDef := range schema.Properties {
		field := api.Field{
			Name:     propName,
			Type:     s.mapPropertyTypeToTypesense(propDef.Type),
			Optional: boolPtr(!propDef.Required),
		}

		if propDef.Type == "vector" && propDef.VectorDim > 0 {
			field.Type = "float[]"
			field.NumDim = pointer.Int(propDef.VectorDim)
		}

		if propDef.Type == "array" && propDef.ElementType != "" {
			field.Type = s.mapPropertyTypeToTypesense(propDef.ElementType) + "[]"
		}

		// Make string, number, and boolean fields facetable by default
		if propDef.Type == "string" || propDef.Type == "number" || propDef.Type == "boolean" {
			field.Facet = boolPtr(true)
		}

		fields = append(fields, field)
	}

	fields = append(fields, api.Field{
		Name:  "entity_type",
		Type:  "string",
		Facet: boolPtr(true),
	})

	return fields
}

func (s *TypesenseStore) mapPropertyTypeToTypesense(propType string) string {
	switch propType {
	case "string":
		return "string"
	case "number":
		return "float"
	case "boolean":
		return "bool"
	case "datetime":
		return "int64"
	case "object":
		return "object"
	case "array":
		return "string[]"
	case "vector":
		return "float[]"
	default:
		return "string"
	}
}

func (s *TypesenseStore) buildFilterString(filters map[string]any) string {
	var filterParts []string

	for field, value := range filters {
		switch v := value.(type) {
		case string:
			filterParts = append(filterParts, fmt.Sprintf("%s:=%s", field, v))
		case int, int64, float64:
			filterParts = append(filterParts, fmt.Sprintf("%s:=%v", field, v))
		case bool:
			filterParts = append(filterParts, fmt.Sprintf("%s:=%v", field, v))
		case []any:

			values := make([]string, len(v))
			for i, val := range v {
				values[i] = fmt.Sprintf("%v", val)
			}
			filterParts = append(filterParts, fmt.Sprintf("%s:[%s]", field, strings.Join(values, ",")))
		case map[string]any:
			// Handle range filters
			if min, ok := v[">="]; ok {
				filterParts = append(filterParts, fmt.Sprintf("%s:>=%v", field, min))
			}
			if max, ok := v["<="]; ok {
				filterParts = append(filterParts, fmt.Sprintf("%s:<=%v", field, max))
			}
			// Also support min/max format
			if min, ok := v["min"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("%s:>=%v", field, min))
			}
			if max, ok := v["max"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("%s:<=%v", field, max))
			}
		}
	}

	return strings.Join(filterParts, " && ")
}

func (s *TypesenseStore) buildSortString(sortOptions []models.SortOption) string {
	var sortParts []string

	for _, sort := range sortOptions {
		direction := "asc"
		if sort.Order == models.SortDesc {
			direction = "desc"
		}
		sortParts = append(sortParts, fmt.Sprintf("%s:%s", sort.Field, direction))
	}

	return strings.Join(sortParts, ",")
}

func (s *TypesenseStore) vectorToString(vector []float32) string {
	strValues := make([]string, len(vector))
	for i, v := range vector {
		strValues[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return "[" + strings.Join(strValues, ",") + "]"
}

func (s *TypesenseStore) convertSearchResult(tsResult *api.SearchResult, query any, searchTime time.Duration) *models.SearchResult {
	var hitCount int
	var totalHits int64
	
	if tsResult.Hits != nil {
		hitCount = len(*tsResult.Hits)
	}
	
	if tsResult.Found != nil {
		totalHits = int64(*tsResult.Found)
	}
	
	result := &models.SearchResult{
		Hits:       make([]models.SearchHit, 0, hitCount),
		TotalHits:  totalHits,
		SearchTime: searchTime.Milliseconds(),
		Query:      query,
	}

	if tsResult.Hits != nil {
		for _, tsHit := range *tsResult.Hits {
			hit := models.SearchHit{
				Score: float32(1.0),
			}

		if doc := tsHit.Document; doc != nil {
			docMap := *doc

			if idStr, ok := docMap["id"].(string); ok {
				hit.ID, _ = uuid.Parse(idStr)
			}

			if entityType, ok := docMap["entity_type"].(string); ok {
				hit.EntityType = entityType
			}

			if urn, ok := docMap["urn"].(string); ok {
				hit.URN = urn
			}

			hit.Properties = make(map[string]any)
			for key, value := range docMap {
				if key != "id" && key != "urn" && key != "entity_type" &&
					key != "created_at" && key != "updated_at" {
					hit.Properties[key] = value
				}
			}
		}

		if tsHit.Highlights != nil {
			hit.Highlights = make(map[string][]string)
			for _, highlight := range *tsHit.Highlights {
				if highlight.Field != nil && highlight.Snippets != nil {
					snippets := make([]string, len(*highlight.Snippets))
					copy(snippets, *highlight.Snippets)
					hit.Highlights[*highlight.Field] = snippets
				}
			}
		}

		if tsHit.VectorDistance != nil {

			hit.Score = float32(1.0 - *tsHit.VectorDistance)
		}

		result.Hits = append(result.Hits, hit)
		}
	}

	if tsResult.FacetCounts != nil && len(*tsResult.FacetCounts) > 0 {
		result.Facets = make(map[string][]models.FacetValue)
		for _, facet := range *tsResult.FacetCounts {
			if facet.FieldName != nil && facet.Counts != nil {
				facetValues := make([]models.FacetValue, 0, len(*facet.Counts))
				for _, count := range *facet.Counts {
					if count.Value != nil && count.Count != nil {
						facetValues = append(facetValues, models.FacetValue{
							Value: *count.Value,
							Count: int64(*count.Count),
						})
					}
				}
				result.Facets[*facet.FieldName] = facetValues
			}
		}
	}

	return result
}

func (s *TypesenseStore) multiSearch(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error) {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.multi_search")
		defer span.End()
		span.SetAttributes(
			attribute.String("query", query.Query),
			attribute.Int("entity_types_count", len(query.EntityTypes)),
		)
	}

	searches := make([]api.MultiSearchCollectionParameters, len(query.EntityTypes))

	for i, entityType := range query.EntityTypes {
		collectionName := s.getCollectionName(entityType)
		searchParams := api.MultiSearchCollectionParameters{
			Collection: collectionName,
			Q:          pointer.String(query.Query),
			QueryBy:    pointer.String("*"),
			Page:       pointer.Int(1),
			PerPage:    pointer.Int(query.Limit),
		}

		if len(query.Filters) > 0 {
			filterStr := s.buildFilterString(query.Filters)
			if filterStr != "" {
				searchParams.FilterBy = pointer.String(filterStr)
			}
		}

		searches[i] = searchParams
	}

	startTime := time.Now()
	searchRequests := api.MultiSearchSearchesParameter{
		Searches: searches,
	}
	multiResult, err := s.client.MultiSearch.Perform(ctx, &api.MultiSearchParams{}, searchRequests)
	if err != nil {
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return nil, fmt.Errorf("multi-search failed: %w", err)
	}

	mergedResult := &models.SearchResult{
		Hits:       []models.SearchHit{},
		TotalHits:  0,
		SearchTime: time.Since(startTime).Milliseconds(),
		Query:      query,
		Facets:     make(map[string][]models.FacetValue),
	}

	for i, result := range multiResult.Results {
		if result.Found != nil {
			mergedResult.TotalHits += int64(*result.Found)
		}

		entityType := query.EntityTypes[i]
		converted := s.convertSearchResult(&result, query, 0)
		for j := range converted.Hits {
			converted.Hits[j].EntityType = entityType
		}
		mergedResult.Hits = append(mergedResult.Hits, converted.Hits...)
	}

	if query.Offset < len(mergedResult.Hits) {
		end := query.Offset + query.Limit
		if end > len(mergedResult.Hits) {
			end = len(mergedResult.Hits)
		}
		mergedResult.Hits = mergedResult.Hits[query.Offset:end]
	} else {
		mergedResult.Hits = []models.SearchHit{}
	}

	return mergedResult, nil
}

func (s *TypesenseStore) multiVectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error) {
	var span trace.Span
	if s.tracer != nil {
		ctx, span = s.tracer.Start(ctx, "typesense.multi_vector_search")
		defer span.End()
		span.SetAttributes(
			attribute.String("vector_field", query.VectorField),
			attribute.Int("entity_types_count", len(query.EntityTypes)),
			attribute.Int("top_k", query.TopK),
		)
	}

	searches := make([]api.MultiSearchCollectionParameters, len(query.EntityTypes))
	vectorStr := s.vectorToString(query.Vector)

	for i, entityType := range query.EntityTypes {
		collectionName := s.getCollectionName(entityType)
		searchParams := api.MultiSearchCollectionParameters{
			Collection:  collectionName,
			Q:           pointer.String("*"),
			VectorQuery: pointer.String(fmt.Sprintf("%s:(%s, k:%d)", query.VectorField, vectorStr, query.TopK)),
			Page:        pointer.Int(1),
			PerPage:     pointer.Int(query.TopK),
		}

		if len(query.Filters) > 0 {
			filterStr := s.buildFilterString(query.Filters)
			if filterStr != "" {
				searchParams.FilterBy = pointer.String(filterStr)
			}
		}

		searches[i] = searchParams
	}

	startTime := time.Now()
	searchRequests := api.MultiSearchSearchesParameter{
		Searches: searches,
	}
	multiResult, err := s.client.MultiSearch.Perform(ctx, &api.MultiSearchParams{}, searchRequests)
	if err != nil {
		if s.tracing != nil {
			s.tracing.SetSpanError(span, err)
		}
		return nil, fmt.Errorf("multi-vector-search failed: %w", err)
	}

	allHits := []models.SearchHit{}
	totalFound := int64(0)

	for i, result := range multiResult.Results {
		if result.Found != nil {
			totalFound += int64(*result.Found)
		}

		entityType := query.EntityTypes[i]
		converted := s.convertSearchResult(&result, query, 0)
		for j := range converted.Hits {
			converted.Hits[j].EntityType = entityType
		}
		allHits = append(allHits, converted.Hits...)
	}

	sortedHits := s.sortHitsByScore(allHits)
	if len(sortedHits) > query.TopK {
		sortedHits = sortedHits[:query.TopK]
	}

	if query.MinScore > 0 {
		filteredHits := []models.SearchHit{}
		for _, hit := range sortedHits {
			if hit.Score >= query.MinScore {
				filteredHits = append(filteredHits, hit)
			}
		}
		sortedHits = filteredHits
	}

	return &models.SearchResult{
		Hits:       sortedHits,
		TotalHits:  int64(len(sortedHits)),
		SearchTime: time.Since(startTime).Milliseconds(),
		Query:      query,
	}, nil
}

func (s *TypesenseStore) sortHitsByScore(hits []models.SearchHit) []models.SearchHit {

	n := len(hits)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if hits[j].Score < hits[j+1].Score {
				hits[j], hits[j+1] = hits[j+1], hits[j]
			}
		}
	}
	return hits
}

// calculatePage calculates the page number from offset and limit
func (s *TypesenseStore) calculatePage(offset, limit int) int {
	if limit <= 0 {
		limit = 1
	}
	return (offset / limit) + 1
}

// inferPropertyType infers the Typesense property type from a Go value
func (s *TypesenseStore) inferPropertyType(value any) string {
	switch v := value.(type) {
	case string:
		return "string"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	case float32, float64:
		return "number"
	case bool:
		return "boolean"
	case []float32, []float64:
		return "vector"
	case []string:
		return "array"  // String array
	case []int, []int32, []int64:
		return "array"  // Number array
	case []map[string]any:
		return "object"  // Array of objects stored as JSON
	case []any:
		if len(v) > 0 {
			// Check if it's an array of objects
			if _, ok := v[0].(map[string]any); ok {
				return "object"  // Typesense will store as JSON string
			}
			// Otherwise, regular array
			return "array"
		}
		return "array"
	case map[string]any:
		return "object"
	case time.Time:
		return "datetime"
	default:
		return "string"
	}
}

var _ store.IndexStore = (*TypesenseStore)(nil)
