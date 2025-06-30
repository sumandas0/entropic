package main

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/sumandas0/entropic/pkg/sdk"
)

func main() {
	// Create a new client
	client, err := sdk.NewClient("http://localhost:8080")
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	ctx := context.Background()

	// Check service health
	if err := client.HealthCheck(ctx); err != nil {
		log.Fatal("Service is not healthy:", err)
	}

	// Example 1: Create an entity schema
	fmt.Println("=== Creating Entity Schema ===")
	userSchema := sdk.NewEntitySchemaBuilder("user").
		AddStringProperty("name", true, true, true).
		AddStringProperty("email", true, true, false).
		AddNumberProperty("age", false, true, true).
		AddVectorProperty("embedding", 384, false).
		AddIndex("email_idx", []string{"email"}, true).
		Build()

	createdSchema, err := client.Schemas.CreateEntitySchema(ctx, userSchema)
	if err != nil {
		log.Printf("Failed to create schema: %v", err)
	} else {
		fmt.Printf("Created schema for entity type: %s\n", createdSchema.EntityType)
	}

	// Example 2: Create an entity
	fmt.Println("\n=== Creating Entity ===")
	userEntity := sdk.NewEntityBuilder().
		WithURN("urn:entropic:user:john.doe").
		WithProperty("name", "John Doe").
		WithProperty("email", "john.doe@example.com").
		WithProperty("age", 30).
		Build()

	createdUser, err := client.Entities.Create(ctx, "user", userEntity)
	if err != nil {
		log.Printf("Failed to create entity: %v", err)
	} else {
		fmt.Printf("Created user with ID: %s\n", createdUser.ID)
	}

	// Example 3: Get an entity
	if createdUser != nil {
		fmt.Println("\n=== Getting Entity ===")
		retrievedUser, err := client.Entities.Get(ctx, "user", createdUser.ID)
		if err != nil {
			log.Printf("Failed to get entity: %v", err)
		} else {
			fmt.Printf("Retrieved user: %s (email: %s)\n", 
				retrievedUser.Properties["name"], 
				retrievedUser.Properties["email"])
		}
	}

	// Example 4: Update an entity
	if createdUser != nil {
		fmt.Println("\n=== Updating Entity ===")
		updateReq := &sdk.EntityRequest{
			URN: createdUser.URN,
			Properties: map[string]interface{}{
				"name":  "John Doe Updated",
				"email": createdUser.Properties["email"],
				"age":   31,
			},
		}

		updatedUser, err := client.Entities.Update(ctx, "user", createdUser.ID, updateReq)
		if err != nil {
			log.Printf("Failed to update entity: %v", err)
		} else {
			fmt.Printf("Updated user age to: %v\n", updatedUser.Properties["age"])
		}
	}

	// Example 5: Create another entity for relationships
	fmt.Println("\n=== Creating Document Entity ===")
	docEntity := sdk.NewEntityBuilder().
		WithURN("urn:entropic:document:report123").
		WithProperty("title", "Annual Report 2023").
		WithProperty("content", "This is the annual report content...").
		Build()

	createdDoc, err := client.Entities.Create(ctx, "document", docEntity)
	if err != nil {
		// Create schema first if it doesn't exist
		docSchema := sdk.NewEntitySchemaBuilder("document").
			AddStringProperty("title", true, true, true).
			AddStringProperty("content", true, false, true).
			Build()
		
		_, schemaErr := client.Schemas.CreateEntitySchema(ctx, docSchema)
		if schemaErr != nil {
			log.Printf("Failed to create document schema: %v", schemaErr)
		} else {
			// Retry entity creation
			createdDoc, err = client.Entities.Create(ctx, "document", docEntity)
			if err != nil {
				log.Printf("Failed to create document: %v", err)
			}
		}
	}

	if createdDoc != nil {
		fmt.Printf("Created document with ID: %s\n", createdDoc.ID)
	}

	// Example 6: Create a relationship
	if createdUser != nil && createdDoc != nil {
		fmt.Println("\n=== Creating Relationship ===")
		
		// First create the relationship schema
		relSchema := &sdk.RelationshipSchemaRequest{
			RelationshipType: "owns",
			FromEntityType:   "user",
			ToEntityType:     "document",
			Cardinality:      sdk.CardinalityOneToMany,
		}
		
		_, err := client.Schemas.CreateRelationshipSchema(ctx, relSchema)
		if err != nil && !isAlreadyExists(err) {
			log.Printf("Failed to create relationship schema: %v", err)
		}

		// Create the relationship
		relation := sdk.NewRelationBuilder("owns").
			From("user", createdUser.ID).
			To("document", createdDoc.ID).
			WithProperty("access_level", "owner").
			Build()

		createdRel, err := client.Relations.Create(ctx, relation)
		if err != nil {
			log.Printf("Failed to create relation: %v", err)
		} else {
			fmt.Printf("Created relation: user %s owns document %s\n", 
				createdRel.FromEntityID, createdRel.ToEntityID)
		}
	}

	// Example 7: List entities
	fmt.Println("\n=== Listing Entities ===")
	userList, err := client.Entities.List(ctx, "user", &sdk.ListOptions{
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		log.Printf("Failed to list entities: %v", err)
	} else {
		fmt.Printf("Found %d users (total: %d)\n", len(userList.Entities), userList.Total)
		for _, user := range userList.Entities {
			fmt.Printf("  - %s (%s)\n", user.Properties["name"], user.URN)
		}
	}

	// Example 8: Search entities
	fmt.Println("\n=== Searching Entities ===")
	searchQuery := sdk.NewSearchQueryBuilder([]string{"user"}, "john").
		WithLimit(10).
		Build()

	searchResults, err := client.Search.Search(ctx, searchQuery)
	if err != nil {
		log.Printf("Failed to search: %v", err)
	} else {
		fmt.Printf("Found %d results in %dms\n", searchResults.TotalHits, searchResults.SearchTime)
		for _, hit := range searchResults.Hits {
			fmt.Printf("  - %s (score: %.2f)\n", hit.Properties["name"], hit.Score)
		}
	}

	// Example 9: Get entity relations
	if createdUser != nil {
		fmt.Println("\n=== Getting Entity Relations ===")
		relations, err := client.Entities.GetRelations(ctx, "user", createdUser.ID, []string{"owns"})
		if err != nil {
			log.Printf("Failed to get relations: %v", err)
		} else {
			fmt.Printf("User has %d relations\n", len(relations))
			for _, rel := range relations {
				fmt.Printf("  - %s -> %s (%s)\n", rel.FromEntityID, rel.ToEntityID, rel.RelationType)
			}
		}
	}

	// Example 10: Error handling
	fmt.Println("\n=== Error Handling Example ===")
	_, err = client.Entities.Get(ctx, "user", uuid.New())
	if err != nil {
		if apiErr, ok := sdk.AsAPIError(err); ok {
			if apiErr.IsNotFound() {
				fmt.Println("Entity not found (as expected)")
			} else {
				fmt.Printf("API Error: %s (type: %s)\n", apiErr.Message, apiErr.Type)
			}
		} else {
			fmt.Printf("Unexpected error: %v\n", err)
		}
	}

	// Cleanup (optional)
	if createdUser != nil {
		fmt.Println("\n=== Cleanup ===")
		err := client.Entities.Delete(ctx, "user", createdUser.ID)
		if err != nil {
			log.Printf("Failed to delete user: %v", err)
		} else {
			fmt.Println("Deleted user entity")
		}
	}

	if createdDoc != nil {
		err := client.Entities.Delete(ctx, "document", createdDoc.ID)
		if err != nil {
			log.Printf("Failed to delete document: %v", err)
		} else {
			fmt.Println("Deleted document entity")
		}
	}

	fmt.Println("\nExample completed!")
}

func isAlreadyExists(err error) bool {
	if apiErr, ok := sdk.AsAPIError(err); ok {
		return apiErr.IsAlreadyExists()
	}
	return false
}