package typesense

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/internal/store"
	"github.com/entropic/entropic/pkg/utils"
	"github.com/google/uuid"
	"github.com/typesense/typesense-go/typesense"
	"github.com/typesense/typesense-go/typesense/api"
	"github.com/typesense/typesense-go/typesense/api/pointer"
)

// TypesenseStore implements the IndexStore interface for Typesense
type TypesenseStore struct {
	client *typesense.Client
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// NewTypesenseStore creates a new Typesense store
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

// IndexEntity indexes an entity in Typesense
func (s *TypesenseStore) IndexEntity(ctx context.Context, entity *models.Entity) error {
	collectionName := s.getCollectionName(entity.EntityType)
	
	// Flatten entity for indexing
	document := s.flattenEntity(entity)
	
	// Upsert document
	_, err := s.client.Collection(collectionName).Documents().Upsert(ctx, document)
	if err != nil {
		return fmt.Errorf("failed to index entity: %w", err)
	}
	
	return nil
}

// UpdateEntityIndex updates an entity in the index
func (s *TypesenseStore) UpdateEntityIndex(ctx context.Context, entity *models.Entity) error {
	// Typesense upsert handles both insert and update
	return s.IndexEntity(ctx, entity)
}

// DeleteEntityIndex removes an entity from the index
func (s *TypesenseStore) DeleteEntityIndex(ctx context.Context, entityType string, id uuid.UUID) error {
	collectionName := s.getCollectionName(entityType)
	
	_, err := s.client.Collection(collectionName).Document(id.String()).Delete(ctx)
	if err != nil {
		// Check if it's a not found error
		if strings.Contains(err.Error(), "not found") {
			return nil // Already deleted, not an error
		}
		return fmt.Errorf("failed to delete entity from index: %w", err)
	}
	
	return nil
}

// Search performs a text search
func (s *TypesenseStore) Search(ctx context.Context, query *models.SearchQuery) (*models.SearchResult, error) {
	if len(query.EntityTypes) == 0 {
		return nil, utils.NewAppError(utils.CodeInvalidInput, "at least one entity type is required", nil)
	}
	
	// For multiple entity types, we need to perform multi-search
	if len(query.EntityTypes) > 1 {
		return s.multiSearch(ctx, query)
	}
	
	// Single entity type search
	collectionName := s.getCollectionName(query.EntityTypes[0])
	
	searchParams := &api.SearchCollectionParams{
		Q:          query.Query,
		QueryBy:    "*", // Search all fields
		Page:       pointer.Int(query.Offset/query.Limit + 1),
		PerPage:    pointer.Int(query.Limit),
	}
	
	// Add filters
	if len(query.Filters) > 0 {
		filterStr := s.buildFilterString(query.Filters)
		if filterStr != "" {
			searchParams.FilterBy = pointer.String(filterStr)
		}
	}
	
	// Add sorting
	if len(query.Sort) > 0 {
		sortStr := s.buildSortString(query.Sort)
		searchParams.SortBy = pointer.String(sortStr)
	}
	
	// Add facets
	if len(query.Facets) > 0 {
		searchParams.FacetBy = pointer.String(strings.Join(query.Facets, ","))
	}
	
	startTime := time.Now()
	result, err := s.client.Collection(collectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	
	return s.convertSearchResult(result, query, time.Since(startTime)), nil
}

// VectorSearch performs a vector similarity search
func (s *TypesenseStore) VectorSearch(ctx context.Context, query *models.VectorQuery) (*models.SearchResult, error) {
	if len(query.EntityTypes) == 0 {
		return nil, utils.NewAppError(utils.CodeInvalidInput, "at least one entity type is required", nil)
	}
	
	// For multiple entity types, we need to perform multi-search
	if len(query.EntityTypes) > 1 {
		return s.multiVectorSearch(ctx, query)
	}
	
	// Single entity type vector search
	collectionName := s.getCollectionName(query.EntityTypes[0])
	
	// Convert vector to string format required by Typesense
	vectorStr := s.vectorToString(query.Vector)
	
	searchParams := &api.SearchCollectionParams{
		Q:              "*", // Vector search doesn't use text query
		VectorQuery:    pointer.String(fmt.Sprintf("%s:(%s, k:%d)", query.VectorField, vectorStr, query.TopK)),
		Page:           pointer.Int(1),
		PerPage:        pointer.Int(query.TopK),
		IncludeFields:  pointer.String("*"),
		ExcludeFields:  pointer.String(""), // Include all fields by default
	}
	
	// Add filters
	if len(query.Filters) > 0 {
		filterStr := s.buildFilterString(query.Filters)
		if filterStr != "" {
			searchParams.FilterBy = pointer.String(filterStr)
		}
	}
	
	// Include vectors if requested
	if !query.IncludeVectors {
		searchParams.ExcludeFields = pointer.String(query.VectorField)
	}
	
	startTime := time.Now()
	result, err := s.client.Collection(collectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}
	
	searchResult := s.convertSearchResult(result, query, time.Since(startTime))
	
	// Filter by minimum score if specified
	if query.MinScore > 0 {
		filteredHits := []models.SearchHit{}
		for _, hit := range searchResult.Hits {
			if hit.Score >= query.MinScore {
				filteredHits = append(filteredHits, hit)
			}
		}
		searchResult.Hits = filteredHits
		searchResult.TotalHits = int64(len(filteredHits))
	}
	
	return searchResult, nil
}

// CreateCollection creates a new collection with the given schema
func (s *TypesenseStore) CreateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error {
	collectionName := s.getCollectionName(entityType)
	
	// Build fields from schema
	fields := s.buildFieldsFromSchema(schema)
	
	// Add default fields
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
		Name:   collectionName,
		Fields: fields,
		DefaultSortingField: stringPtr("updated_at"),
		EnableNestedFields: boolPtr(true),
	}
	
	_, err := s.client.Collections().Create(ctx, collectionSchema)
	if err != nil {
		// Check if collection already exists
		if strings.Contains(err.Error(), "already exists") {
			// Update the collection instead
			return s.UpdateCollection(ctx, entityType, schema)
		}
		return fmt.Errorf("failed to create collection: %w", err)
	}
	
	return nil
}

// UpdateCollection updates an existing collection schema
func (s *TypesenseStore) UpdateCollection(ctx context.Context, entityType string, schema *models.EntitySchema) error {
	collectionName := s.getCollectionName(entityType)
	
	// Get existing collection
	existingCollection, err := s.client.Collection(collectionName).Retrieve(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Collection doesn't exist, create it
			return s.CreateCollection(ctx, entityType, schema)
		}
		return fmt.Errorf("failed to retrieve collection: %w", err)
	}
	
	// Build new fields from schema
	newFields := s.buildFieldsFromSchema(schema)
	
	// Find fields that need to be added
	existingFieldMap := make(map[string]bool)
	for _, field := range existingCollection.Fields {
		existingFieldMap[field.Name] = true
	}
	
	// Add new fields
	for _, field := range newFields {
		if !existingFieldMap[field.Name] {
			updateSchema := &api.CollectionUpdateSchema{
				Fields: []api.Field{field},
			}
			_, err := s.client.Collection(collectionName).Update(ctx, updateSchema)
			if err != nil {
				return fmt.Errorf("failed to add field %s: %w", field.Name, err)
			}
		}
	}
	
	return nil
}

// DeleteCollection deletes a collection
func (s *TypesenseStore) DeleteCollection(ctx context.Context, entityType string) error {
	collectionName := s.getCollectionName(entityType)
	
	_, err := s.client.Collection(collectionName).Delete(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete collection: %w", err)
	}
	
	return nil
}

// Ping checks if Typesense is reachable
func (s *TypesenseStore) Ping(ctx context.Context) error {
	_, err := s.client.Health(ctx, 5*time.Second)
	return err
}

// Close closes the connection (no-op for Typesense HTTP client)
func (s *TypesenseStore) Close() error {
	return nil
}

// Helper methods

func (s *TypesenseStore) getCollectionName(entityType string) string {
	// Replace spaces and special characters with underscores
	name := strings.ToLower(entityType)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return "entity_" + name
}

func (s *TypesenseStore) flattenEntity(entity *models.Entity) map[string]interface{} {
	doc := make(map[string]interface{})
	
	// Add base fields
	doc["id"] = entity.ID.String()
	doc["urn"] = entity.URN
	doc["entity_type"] = entity.EntityType
	doc["created_at"] = entity.CreatedAt.Unix()
	doc["updated_at"] = entity.UpdatedAt.Unix()
	
	// Flatten properties
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
		
		// Handle vector fields
		if propDef.Type == "vector" && propDef.VectorDim > 0 {
			field.Type = fmt.Sprintf("float[]")
			field.NumDim = pointer.Int(propDef.VectorDim)
		}
		
		// Handle array fields
		if propDef.Type == "array" && propDef.ElementType != "" {
			field.Type = s.mapPropertyTypeToTypesense(propDef.ElementType) + "[]"
		}
		
		// Add faceting for certain types
		if propDef.Type == "string" || propDef.Type == "number" || propDef.Type == "boolean" {
			field.Facet = boolPtr(true)
		}
		
		fields = append(fields, field)
	}
	
	// Add entity_type as a faceted field
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
		return "int64" // Unix timestamp
	case "object":
		return "object"
	case "array":
		return "string[]" // Default array type
	case "vector":
		return "float[]"
	default:
		return "string"
	}
}

func (s *TypesenseStore) buildFilterString(filters map[string]interface{}) string {
	var filterParts []string
	
	for field, value := range filters {
		switch v := value.(type) {
		case string:
			filterParts = append(filterParts, fmt.Sprintf("%s:=%s", field, v))
		case int, int64, float64:
			filterParts = append(filterParts, fmt.Sprintf("%s:=%v", field, v))
		case bool:
			filterParts = append(filterParts, fmt.Sprintf("%s:=%v", field, v))
		case []interface{}:
			// Handle array filters (OR condition)
			values := make([]string, len(v))
			for i, val := range v {
				values[i] = fmt.Sprintf("%v", val)
			}
			filterParts = append(filterParts, fmt.Sprintf("%s:[%s]", field, strings.Join(values, ",")))
		case map[string]interface{}:
			// Handle range filters
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

func (s *TypesenseStore) convertSearchResult(tsResult *api.SearchResult, query interface{}, searchTime time.Duration) *models.SearchResult {
	result := &models.SearchResult{
		Hits:       make([]models.SearchHit, 0, len(*tsResult.Hits)),
		TotalHits:  int64(*tsResult.Found),
		SearchTime: searchTime,
		Query:      query,
	}
	
	// Convert hits
	for _, tsHit := range *tsResult.Hits {
		hit := models.SearchHit{
			Score: float32(1.0), // Default score
		}
		
		// Extract document fields
		if doc := tsHit.Document; doc != nil {
			docMap := *doc
			
			// Extract ID
			if idStr, ok := docMap["id"].(string); ok {
				hit.ID, _ = uuid.Parse(idStr)
			}
			
			// Extract entity type
			if entityType, ok := docMap["entity_type"].(string); ok {
				hit.EntityType = entityType
			}
			
			// Extract URN
			if urn, ok := docMap["urn"].(string); ok {
				hit.URN = urn
			}
			
			// Extract properties (exclude system fields)
			hit.Properties = make(map[string]interface{})
			for key, value := range docMap {
				if key != "id" && key != "urn" && key != "entity_type" && 
				   key != "created_at" && key != "updated_at" {
					hit.Properties[key] = value
				}
			}
		}
		
		// Extract highlights
		if tsHit.Highlights != nil {
			hit.Highlights = make(map[string][]string)
			for _, highlight := range *tsHit.Highlights {
				if highlight.Field != nil && highlight.Snippets != nil {
					snippets := make([]string, len(*highlight.Snippets))
					for i, snippet := range *highlight.Snippets {
						snippets[i] = snippet
					}
					hit.Highlights[*highlight.Field] = snippets
				}
			}
		}
		
		// Extract text match score if available
		// TODO: Check the actual field name for text match score in the API
		// For now, we'll use a default score
		
		// Extract vector distance if available
		if tsHit.VectorDistance != nil {
			// Convert distance to similarity score (1 - distance for cosine similarity)
			hit.Score = float32(1.0 - *tsHit.VectorDistance)
		}
		
		result.Hits = append(result.Hits, hit)
	}
	
	// Convert facets
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
	// Build multi-search request
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
		
		// Add filters
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
		return nil, fmt.Errorf("multi-search failed: %w", err)
	}
	
	// Merge results
	mergedResult := &models.SearchResult{
		Hits:       []models.SearchHit{},
		TotalHits:  0,
		SearchTime: time.Since(startTime),
		Query:      query,
		Facets:     make(map[string][]models.FacetValue),
	}
	
	for i, result := range multiResult.Results {
		if result.Found != nil {
			mergedResult.TotalHits += int64(*result.Found)
		}
		
		// Set entity type for all hits from this collection
		entityType := query.EntityTypes[i]
		converted := s.convertSearchResult(&result, query, 0)
		for j := range converted.Hits {
			converted.Hits[j].EntityType = entityType
		}
		mergedResult.Hits = append(mergedResult.Hits, converted.Hits...)
	}
	
	// Apply limit and offset to merged results
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
	// Build multi-search request for vector search
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
		
		// Add filters
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
		return nil, fmt.Errorf("multi-vector-search failed: %w", err)
	}
	
	// Merge and sort results by score
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
	
	// Sort by score descending and apply TopK limit
	sortedHits := s.sortHitsByScore(allHits)
	if len(sortedHits) > query.TopK {
		sortedHits = sortedHits[:query.TopK]
	}
	
	// Filter by minimum score if specified
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
		SearchTime: time.Since(startTime),
		Query:      query,
	}, nil
}

func (s *TypesenseStore) sortHitsByScore(hits []models.SearchHit) []models.SearchHit {
	// Simple bubble sort for small result sets
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

// Ensure TypesenseStore implements IndexStore
var _ store.IndexStore = (*TypesenseStore)(nil)