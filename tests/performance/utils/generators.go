package utils

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sumandas0/entropic/internal/models"
)

// DataGenerator generates test data for performance testing
type DataGenerator struct {
	rand   *rand.Rand
	mu     sync.Mutex
	seed   int64
	idGen  int64
}

// EntityTemplate defines a template for generating entities
type EntityTemplate struct {
	Type           string
	PropertySizeKB int
	IncludeVector  bool
	VectorDim      int
	ComplexityLevel string // simple, medium, complex
}

// LoadPattern defines how load should be generated
type LoadPattern struct {
	Type        string        // steady, burst, ramp, spike
	BaseRate    int           // operations per second
	Duration    time.Duration
	BurstFactor float64       // for burst pattern
	RampUpTime  time.Duration // for ramp pattern
}

// NewDataGenerator creates a new data generator
func NewDataGenerator(seed int64) *DataGenerator {
	return &DataGenerator{
		rand: rand.New(rand.NewSource(seed)),
		seed: seed,
	}
}

// GenerateEntity creates a test entity based on template
func (dg *DataGenerator) GenerateEntity(template EntityTemplate) *models.Entity {
	id := atomic.AddInt64(&dg.idGen, 1)
	urn := fmt.Sprintf("perf:%s:%d:%d", template.Type, dg.seed, id)
	
	properties := dg.generateProperties(template)
	
	return models.NewEntity(template.Type, urn, properties)
}

// GenerateBatch creates a batch of entities
func (dg *DataGenerator) GenerateBatch(template EntityTemplate, count int) []*models.Entity {
	entities := make([]*models.Entity, count)
	for i := 0; i < count; i++ {
		entities[i] = dg.GenerateEntity(template)
	}
	return entities
}

// GenerateRelation creates a test relation between entities
func (dg *DataGenerator) GenerateRelation(relationType string, from, to *models.Entity) *models.Relation {
	properties := map[string]interface{}{
		"weight":     dg.randomFloat(0, 1),
		"created_at": time.Now().Unix(),
		"metadata": map[string]interface{}{
			"source": "performance_test",
			"seed":   dg.seed,
		},
	}
	
	return models.NewRelation(
		relationType,
		from.ID,
		from.EntityType,
		to.ID,
		to.EntityType,
		properties,
	)
}

// generateProperties creates properties based on template
func (dg *DataGenerator) generateProperties(template EntityTemplate) map[string]interface{} {
	properties := make(map[string]interface{})
	
	switch template.ComplexityLevel {
	case "simple":
		properties = dg.generateSimpleProperties()
	case "medium":
		properties = dg.generateMediumProperties()
	case "complex":
		properties = dg.generateComplexProperties()
	default:
		properties = dg.generateMediumProperties()
	}
	
	// Add vector if requested
	if template.IncludeVector {
		properties["embedding"] = dg.generateVector(template.VectorDim)
	}
	
	// Pad to desired size
	dg.padToSize(properties, template.PropertySizeKB)
	
	return properties
}

func (dg *DataGenerator) generateSimpleProperties() map[string]interface{} {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	return map[string]interface{}{
		"name":        dg.randomString(20),
		"description": dg.randomString(100),
		"value":       dg.randomFloat(0, 1000),
		"active":      dg.rand.Intn(2) == 1,
		"created_at":  time.Now().Unix(),
	}
}

func (dg *DataGenerator) generateMediumProperties() map[string]interface{} {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	tags := make([]string, 5)
	for i := 0; i < 5; i++ {
		tags[i] = dg.randomString(10)
	}
	
	return map[string]interface{}{
		"name":        dg.randomString(30),
		"description": dg.randomString(200),
		"category":    dg.randomChoice([]string{"electronics", "clothing", "books", "food", "toys"}),
		"price":       dg.randomFloat(10, 1000),
		"quantity":    dg.rand.Intn(1000),
		"tags":        tags,
		"metadata": map[string]interface{}{
			"brand":      dg.randomString(20),
			"model":      dg.randomString(15),
			"year":       2020 + dg.rand.Intn(5),
			"warranty":   dg.rand.Intn(5),
			"dimensions": map[string]float64{
				"length": dg.randomFloat(1, 100),
				"width":  dg.randomFloat(1, 100),
				"height": dg.randomFloat(1, 100),
			},
		},
		"active":     dg.rand.Intn(2) == 1,
		"created_at": time.Now().Unix(),
		"updated_at": time.Now().Unix(),
	}
}

func (dg *DataGenerator) generateComplexProperties() map[string]interface{} {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	// Generate nested structure
	properties := dg.generateMediumProperties()
	
	// Add more complex nested data
	properties["specifications"] = dg.generateSpecifications()
	properties["inventory"] = dg.generateInventory()
	properties["reviews"] = dg.generateReviews(3)
	properties["attributes"] = dg.generateAttributes(10)
	
	return properties
}

func (dg *DataGenerator) generateSpecifications() map[string]interface{} {
	specs := make(map[string]interface{})
	specCount := 5 + dg.rand.Intn(10)
	
	for i := 0; i < specCount; i++ {
		key := fmt.Sprintf("spec_%d", i)
		specs[key] = map[string]interface{}{
			"name":  dg.randomString(20),
			"value": dg.randomString(30),
			"unit":  dg.randomChoice([]string{"mm", "kg", "GB", "MHz", "inch"}),
		}
	}
	
	return specs
}

func (dg *DataGenerator) generateInventory() map[string]interface{} {
	locations := make([]map[string]interface{}, 3)
	for i := 0; i < 3; i++ {
		locations[i] = map[string]interface{}{
			"warehouse_id": uuid.New().String(),
			"quantity":     dg.rand.Intn(100),
			"reserved":     dg.rand.Intn(20),
			"location":     fmt.Sprintf("A%d-B%d", dg.rand.Intn(10), dg.rand.Intn(100)),
		}
	}
	
	return map[string]interface{}{
		"total_quantity": dg.rand.Intn(1000),
		"available":      dg.rand.Intn(800),
		"reserved":       dg.rand.Intn(200),
		"locations":      locations,
		"last_restocked": time.Now().Add(-time.Duration(dg.rand.Intn(30)) * 24 * time.Hour).Unix(),
	}
}

func (dg *DataGenerator) generateReviews(count int) []map[string]interface{} {
	reviews := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		reviews[i] = map[string]interface{}{
			"user_id":    uuid.New().String(),
			"rating":     1 + dg.rand.Intn(5),
			"title":      dg.randomString(50),
			"comment":    dg.randomString(200),
			"helpful":    dg.rand.Intn(100),
			"verified":   dg.rand.Intn(2) == 1,
			"created_at": time.Now().Add(-time.Duration(dg.rand.Intn(365)) * 24 * time.Hour).Unix(),
		}
	}
	return reviews
}

func (dg *DataGenerator) generateAttributes(count int) map[string]interface{} {
	attrs := make(map[string]interface{})
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("attr_%s", dg.randomString(10))
		attrs[key] = dg.randomValue()
	}
	return attrs
}

func (dg *DataGenerator) generateVector(dim int) []float32 {
	vector := make([]float32, dim)
	for i := 0; i < dim; i++ {
		vector[i] = dg.rand.Float32()
	}
	return vector
}

func (dg *DataGenerator) padToSize(properties map[string]interface{}, targetKB int) {
	// Estimate current size (rough approximation)
	currentSize := len(fmt.Sprintf("%v", properties))
	targetSize := targetKB * 1024
	
	if currentSize < targetSize {
		padding := targetSize - currentSize
		properties["_padding"] = dg.randomString(padding)
	}
}

// Utility functions

func (dg *DataGenerator) randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[dg.rand.Intn(len(charset))]
	}
	return string(result)
}

func (dg *DataGenerator) randomFloat(min, max float64) float64 {
	return min + dg.rand.Float64()*(max-min)
}

func (dg *DataGenerator) randomChoice(choices []string) string {
	return choices[dg.rand.Intn(len(choices))]
}

func (dg *DataGenerator) randomValue() interface{} {
	switch dg.rand.Intn(4) {
	case 0:
		return dg.randomString(20)
	case 1:
		return dg.rand.Intn(1000)
	case 2:
		return dg.randomFloat(0, 100)
	default:
		return dg.rand.Intn(2) == 1
	}
}

// LoadGenerator generates load according to patterns
type LoadGenerator struct {
	pattern LoadPattern
	ticker  *time.Ticker
	done    chan bool
}

// NewLoadGenerator creates a new load generator
func NewLoadGenerator(pattern LoadPattern) *LoadGenerator {
	return &LoadGenerator{
		pattern: pattern,
		done:    make(chan bool),
	}
}

// Start begins generating load
func (lg *LoadGenerator) Start(operation func() error) {
	switch lg.pattern.Type {
	case "steady":
		lg.steadyLoad(operation)
	case "burst":
		lg.burstLoad(operation)
	case "ramp":
		lg.rampLoad(operation)
	case "spike":
		lg.spikeLoad(operation)
	default:
		lg.steadyLoad(operation)
	}
}

// Stop halts load generation
func (lg *LoadGenerator) Stop() {
	close(lg.done)
	if lg.ticker != nil {
		lg.ticker.Stop()
	}
}

func (lg *LoadGenerator) steadyLoad(operation func() error) {
	interval := time.Second / time.Duration(lg.pattern.BaseRate)
	lg.ticker = time.NewTicker(interval)
	
	go func() {
		for {
			select {
			case <-lg.ticker.C:
				go operation()
			case <-lg.done:
				return
			}
		}
	}()
}

func (lg *LoadGenerator) burstLoad(operation func() error) {
	normalInterval := time.Second / time.Duration(lg.pattern.BaseRate)
	burstInterval := normalInterval / time.Duration(lg.pattern.BurstFactor)
	
	go func() {
		burstTicker := time.NewTicker(10 * time.Second) // Burst every 10 seconds
		defer burstTicker.Stop()
		
		for {
			select {
			case <-burstTicker.C:
				// Generate burst
				for i := 0; i < int(float64(lg.pattern.BaseRate)*lg.pattern.BurstFactor); i++ {
					go operation()
					time.Sleep(burstInterval)
				}
			case <-lg.done:
				return
			default:
				// Normal load
				go operation()
				time.Sleep(normalInterval)
			}
		}
	}()
}

func (lg *LoadGenerator) rampLoad(operation func() error) {
	go func() {
		startTime := time.Now()
		for {
			select {
			case <-lg.done:
				return
			default:
				elapsed := time.Since(startTime)
				if elapsed > lg.pattern.RampUpTime {
					elapsed = lg.pattern.RampUpTime
				}
				
				// Calculate current rate based on ramp
				progress := float64(elapsed) / float64(lg.pattern.RampUpTime)
				currentRate := int(float64(lg.pattern.BaseRate) * progress)
				if currentRate < 1 {
					currentRate = 1
				}
				
				interval := time.Second / time.Duration(currentRate)
				go operation()
				time.Sleep(interval)
			}
		}
	}()
}

func (lg *LoadGenerator) spikeLoad(operation func() error) {
	go func() {
		spikeTicker := time.NewTicker(30 * time.Second) // Spike every 30 seconds
		defer spikeTicker.Stop()
		
		normalInterval := time.Second / time.Duration(lg.pattern.BaseRate)
		
		for {
			select {
			case <-spikeTicker.C:
				// Generate spike (10x normal rate for 5 seconds)
				spikeEnd := time.Now().Add(5 * time.Second)
				spikeInterval := normalInterval / 10
				
				for time.Now().Before(spikeEnd) {
					go operation()
					time.Sleep(spikeInterval)
				}
			case <-lg.done:
				return
			default:
				// Normal load
				go operation()
				time.Sleep(normalInterval)
			}
		}
	}()
}

// GenerateEntityGraph creates a connected graph of entities
func GenerateEntityGraph(gen *DataGenerator, nodeCount int, avgConnections int) ([]*models.Entity, []*models.Relation) {
	// Generate entities
	entities := make([]*models.Entity, nodeCount)
	template := EntityTemplate{
		Type:            "node",
		PropertySizeKB:  1,
		ComplexityLevel: "simple",
	}
	
	for i := 0; i < nodeCount; i++ {
		entities[i] = gen.GenerateEntity(template)
	}
	
	// Generate relations
	relations := make([]*models.Relation, 0, nodeCount*avgConnections/2)
	for i := 0; i < nodeCount; i++ {
		connections := avgConnections/2 + gen.rand.Intn(avgConnections/2)
		for j := 0; j < connections; j++ {
			targetIdx := gen.rand.Intn(nodeCount)
			if targetIdx != i {
				relation := gen.GenerateRelation("connected_to", entities[i], entities[targetIdx])
				relations = append(relations, relation)
			}
		}
	}
	
	return entities, relations
}

// GenerateRealisticEcommerce creates realistic e-commerce test data
func GenerateRealisticEcommerce(gen *DataGenerator) ([]*models.Entity, []*models.Relation) {
	var entities []*models.Entity
	var relations []*models.Relation
	
	// Generate users
	userTemplate := EntityTemplate{
		Type:            "user",
		PropertySizeKB:  2,
		ComplexityLevel: "medium",
	}
	users := gen.GenerateBatch(userTemplate, 100)
	entities = append(entities, users...)
	
	// Generate products
	productTemplate := EntityTemplate{
		Type:            "product",
		PropertySizeKB:  5,
		ComplexityLevel: "complex",
		IncludeVector:   true,
		VectorDim:       384,
	}
	products := gen.GenerateBatch(productTemplate, 500)
	entities = append(entities, products...)
	
	// Generate orders
	orderTemplate := EntityTemplate{
		Type:            "order",
		PropertySizeKB:  3,
		ComplexityLevel: "medium",
	}
	orders := gen.GenerateBatch(orderTemplate, 200)
	entities = append(entities, orders...)
	
	// Create relations
	for _, order := range orders {
		// Order belongs to user
		user := users[gen.rand.Intn(len(users))]
		rel := gen.GenerateRelation("placed_by", order, user)
		relations = append(relations, rel)
		
		// Order contains products
		productCount := 1 + gen.rand.Intn(5)
		for i := 0; i < productCount; i++ {
			product := products[gen.rand.Intn(len(products))]
			rel := gen.GenerateRelation("contains", order, product)
			relations = append(relations, rel)
		}
	}
	
	return entities, relations
}