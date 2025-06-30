package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"

	"github.com/sumandas0/entropic/pkg/sdk"
)

func main() {
	// Create a new client
	client, err := sdk.NewClient("http://localhost:8080")
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	ctx := context.Background()

	// Create schema for documents with vector embeddings
	fmt.Println("=== Setting up Document Schema with Vector Support ===")
	docSchema := sdk.NewEntitySchemaBuilder("article").
		AddStringProperty("title", true, true, true).
		AddStringProperty("content", true, false, true).
		AddStringProperty("category", true, true, true).
		AddVectorProperty("content_embedding", 384, true). // 384-dimensional vectors
		AddNumberProperty("publish_year", true, true, true).
		Build()

	_, err = client.Schemas.CreateEntitySchema(ctx, docSchema)
	if err != nil && !isAlreadyExists(err) {
		log.Printf("Failed to create schema: %v", err)
		return
	}

	// Create sample documents with embeddings
	fmt.Println("\n=== Creating Sample Documents ===")
	articles := []struct {
		title    string
		content  string
		category string
		year     int
	}{
		{
			title:    "Introduction to Machine Learning",
			content:  "Machine learning is a subset of artificial intelligence...",
			category: "AI",
			year:     2023,
		},
		{
			title:    "Deep Learning Fundamentals",
			content:  "Deep learning uses neural networks with multiple layers...",
			category: "AI",
			year:     2023,
		},
		{
			title:    "Natural Language Processing Basics",
			content:  "NLP enables computers to understand human language...",
			category: "AI",
			year:     2024,
		},
		{
			title:    "Computer Vision Applications",
			content:  "Computer vision allows machines to interpret visual data...",
			category: "AI",
			year:     2024,
		},
		{
			title:    "Quantum Computing Overview",
			content:  "Quantum computing leverages quantum mechanics principles...",
			category: "Physics",
			year:     2023,
		},
	}

	createdIDs := []string{}
	
	for i, article := range articles {
		// Generate a mock embedding (in real use, you'd use an embedding model)
		embedding := generateMockEmbedding(384, i)
		
		entity := sdk.NewEntityBuilder().
			WithURN(fmt.Sprintf("urn:entropic:article:%d", i+1)).
			WithProperty("title", article.title).
			WithProperty("content", article.content).
			WithProperty("category", article.category).
			WithProperty("publish_year", article.year).
			WithProperty("content_embedding", embedding).
			Build()

		created, err := client.Entities.Create(ctx, "article", entity)
		if err != nil {
			log.Printf("Failed to create article: %v", err)
			continue
		}
		
		createdIDs = append(createdIDs, created.ID.String())
		fmt.Printf("Created article: %s\n", article.title)
	}

	// Perform vector similarity search
	fmt.Println("\n=== Performing Vector Similarity Search ===")
	
	// Create a query vector (simulating an embedding of a search query)
	// In real use, you'd embed the search query using the same model
	queryVector := generateMockEmbedding(384, 0) // Similar to first article
	
	vectorQuery := sdk.NewVectorQueryBuilder(
		[]string{"article"},
		queryVector,
		"content_embedding",
	).
		WithTopK(3).
		WithMinScore(0.5).
		Build()

	results, err := client.Search.VectorSearch(ctx, vectorQuery)
	if err != nil {
		log.Fatal("Vector search failed:", err)
	}

	fmt.Printf("\nFound %d similar articles:\n", len(results.Hits))
	for i, hit := range results.Hits {
		fmt.Printf("%d. %s (score: %.3f)\n", i+1, hit.Properties["title"], hit.Score)
		fmt.Printf("   Category: %s, Year: %.0f\n", 
			hit.Properties["category"], 
			hit.Properties["publish_year"])
	}

	// Vector search with filters
	fmt.Println("\n=== Vector Search with Filters ===")
	
	filteredQuery := sdk.NewVectorQueryBuilder(
		[]string{"article"},
		queryVector,
		"content_embedding",
	).
		WithTopK(5).
		WithFilter("category", "AI").
		WithFilter("publish_year", 2024).
		Build()

	filteredResults, err := client.Search.VectorSearch(ctx, filteredQuery)
	if err != nil {
		log.Printf("Filtered vector search failed: %v", err)
	} else {
		fmt.Printf("\nFound %d AI articles from 2024:\n", len(filteredResults.Hits))
		for _, hit := range filteredResults.Hits {
			fmt.Printf("- %s (score: %.3f)\n", hit.Properties["title"], hit.Score)
		}
	}

	// Hybrid search (text + vector)
	fmt.Println("\n=== Hybrid Search Example ===")
	
	// First, text search
	textQuery := sdk.NewSearchQueryBuilder([]string{"article"}, "learning").
		WithFilter("category", "AI").
		WithLimit(10).
		Build()

	textResults, err := client.Search.Search(ctx, textQuery)
	if err != nil {
		log.Printf("Text search failed: %v", err)
	} else {
		fmt.Printf("\nText search found %d results for 'learning':\n", textResults.TotalHits)
		for _, hit := range textResults.Hits {
			fmt.Printf("- %s\n", hit.Properties["title"])
		}
	}

	// Advanced: Search with vector similarity for reranking
	fmt.Println("\n=== Advanced: Pagination with Iterator ===")
	
	// Create a search query for pagination
	paginatedQuery := sdk.NewSearchQueryBuilder([]string{"article"}, "").
		WithLimit(2). // Small page size for demo
		Build()

	iterator := client.Search.NewIterator(paginatedQuery)
	pageNum := 1
	
	for iterator.HasMore() {
		page, err := iterator.Next(ctx)
		if err != nil {
			log.Printf("Failed to fetch page: %v", err)
			break
		}
		
		if page == nil {
			break
		}
		
		fmt.Printf("\nPage %d:\n", pageNum)
		for _, hit := range page.Hits {
			fmt.Printf("  - %s\n", hit.Properties["title"])
		}
		pageNum++
	}

	// Cleanup
	fmt.Println("\n=== Cleanup ===")
	for _, id := range createdIDs {
		if uid, err := sdk.ParseUUID(id); err == nil {
			err := client.Entities.Delete(ctx, "article", uid)
			if err != nil {
				log.Printf("Failed to delete article %s: %v", id, err)
			}
		}
	}
	
	fmt.Println("\nVector search example completed!")
}

// generateMockEmbedding creates a mock embedding vector for demonstration
// In real applications, you would use an embedding model like OpenAI, Sentence-Transformers, etc.
func generateMockEmbedding(dim int, seed int) []float32 {
	rand.Seed(int64(seed))
	embedding := make([]float32, dim)
	
	// Generate normalized random vector
	var norm float32
	for i := range embedding {
		embedding[i] = rand.Float32()*2 - 1 // Range [-1, 1]
		norm += embedding[i] * embedding[i]
	}
	
	// Normalize to unit vector
	norm = float32(1.0 / sqrt(float64(norm)))
	for i := range embedding {
		embedding[i] *= norm
	}
	
	return embedding
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func isAlreadyExists(err error) bool {
	if apiErr, ok := sdk.AsAPIError(err); ok {
		return apiErr.IsAlreadyExists()
	}
	return false
}