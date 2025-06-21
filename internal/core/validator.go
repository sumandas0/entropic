package core

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/entropic/entropic/internal/cache"
	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/internal/store"
	"github.com/entropic/entropic/pkg/utils"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// Validator provides schema-based validation for entities and relationships
type Validator struct {
	validate     *validator.Validate
	cacheManager *cache.Manager
	primaryStore store.PrimaryStore
}

// NewValidator creates a new validator instance
func NewValidator(cacheManager *cache.Manager, primaryStore store.PrimaryStore) *Validator {
	validate := validator.New()
	
	v := &Validator{
		validate:     validate,
		cacheManager: cacheManager,
		primaryStore: primaryStore,
	}
	
	// Register custom validation functions
	v.registerCustomValidators()
	
	return v
}

// ValidateEntity validates an entity against its schema
func (v *Validator) ValidateEntity(ctx context.Context, entity *models.Entity) error {
	// Get entity schema
	schema, err := v.cacheManager.GetEntitySchema(ctx, entity.EntityType)
	if err != nil {
		return utils.NewAppError(utils.CodeValidation, "entity schema not found", err).
			WithDetail("entity_type", entity.EntityType)
	}
	
	// Validate basic entity structure
	if err := v.validate.Struct(entity); err != nil {
		return utils.NewAppError(utils.CodeValidation, "entity structure validation failed", err)
	}
	
	// Validate URN uniqueness
	if err := v.validateURNUniqueness(ctx, entity); err != nil {
		return err
	}
	
	// Validate properties against schema
	if err := v.validateProperties(entity.Properties, schema.Properties); err != nil {
		return utils.NewAppError(utils.CodeValidation, "property validation failed", err).
			WithDetail("entity_type", entity.EntityType)
	}
	
	return nil
}

// ValidateRelation validates a relation against its schema
func (v *Validator) ValidateRelation(ctx context.Context, relation *models.Relation) error {
	// Get relationship schema
	schema, err := v.cacheManager.GetRelationshipSchema(ctx, relation.RelationType)
	if err != nil {
		return utils.NewAppError(utils.CodeValidation, "relationship schema not found", err).
			WithDetail("relation_type", relation.RelationType)
	}
	
	// Validate basic relation structure
	if err := v.validate.Struct(relation); err != nil {
		return utils.NewAppError(utils.CodeValidation, "relation structure validation failed", err)
	}
	
	// Validate entity types match schema
	if relation.FromEntityType != schema.FromEntityType {
		return utils.NewAppError(utils.CodeValidation, "from entity type mismatch", nil).
			WithDetail("expected", schema.FromEntityType).
			WithDetail("actual", relation.FromEntityType)
	}
	
	if relation.ToEntityType != schema.ToEntityType {
		return utils.NewAppError(utils.CodeValidation, "to entity type mismatch", nil).
			WithDetail("expected", schema.ToEntityType).
			WithDetail("actual", relation.ToEntityType)
	}
	
	// Validate referenced entities exist
	if err := v.validateEntityReferences(ctx, relation); err != nil {
		return err
	}
	
	// Validate cardinality constraints
	if err := v.validateCardinality(ctx, relation, schema); err != nil {
		return err
	}
	
	// Validate relation properties against schema
	if relation.Properties != nil && len(relation.Properties) > 0 {
		if err := v.validateProperties(relation.Properties, schema.Properties); err != nil {
			return utils.NewAppError(utils.CodeValidation, "relation property validation failed", err).
				WithDetail("relation_type", relation.RelationType)
		}
	}
	
	return nil
}

// ValidateEntitySchema validates an entity schema
func (v *Validator) ValidateEntitySchema(schema *models.EntitySchema) error {
	if err := v.validate.Struct(schema); err != nil {
		return utils.NewAppError(utils.CodeValidation, "entity schema structure validation failed", err)
	}
	
	// Validate property definitions
	for propName, propDef := range schema.Properties {
		if err := v.validatePropertyDefinition(propName, propDef); err != nil {
			return utils.NewAppError(utils.CodeValidation, "property definition validation failed", err).
				WithDetail("property", propName)
		}
	}
	
	// Validate indexes
	for _, index := range schema.Indexes {
		if err := v.validateIndexConfig(index, schema.Properties); err != nil {
			return utils.NewAppError(utils.CodeValidation, "index configuration validation failed", err).
				WithDetail("index", index.Name)
		}
	}
	
	return nil
}

// ValidateRelationshipSchema validates a relationship schema
func (v *Validator) ValidateRelationshipSchema(schema *models.RelationshipSchema) error {
	if err := v.validate.Struct(schema); err != nil {
		return utils.NewAppError(utils.CodeValidation, "relationship schema structure validation failed", err)
	}
	
	// Validate property definitions
	for propName, propDef := range schema.Properties {
		if err := v.validatePropertyDefinition(propName, propDef); err != nil {
			return utils.NewAppError(utils.CodeValidation, "property definition validation failed", err).
				WithDetail("property", propName)
		}
	}
	
	return nil
}

// validateURNUniqueness checks if the URN is unique
func (v *Validator) validateURNUniqueness(ctx context.Context, entity *models.Entity) error {
	exists, err := v.primaryStore.CheckURNExists(ctx, entity.URN)
	if err != nil {
		return utils.NewAppError(utils.CodeInternal, "failed to check URN uniqueness", err)
	}
	
	if exists {
		return utils.NewAppError(utils.CodeAlreadyExists, "URN already exists", nil).
			WithDetail("urn", entity.URN)
	}
	
	return nil
}

// validateProperties validates properties against their schema definitions
func (v *Validator) validateProperties(properties map[string]interface{}, schema models.PropertySchema) error {
	// Check required properties
	for propName, propDef := range schema {
		if propDef.Required {
			if _, exists := properties[propName]; !exists {
				return fmt.Errorf("required property '%s' is missing", propName)
			}
		}
	}
	
	// Validate each property
	for propName, value := range properties {
		propDef, exists := schema[propName]
		if !exists {
			// Allow extra properties by default - could be configurable
			continue
		}
		
		if err := v.validatePropertyValue(propName, value, propDef); err != nil {
			return err
		}
	}
	
	return nil
}

// validatePropertyValue validates a single property value
func (v *Validator) validatePropertyValue(propName string, value interface{}, propDef models.PropertyDefinition) error {
	if value == nil {
		if propDef.Required {
			return fmt.Errorf("property '%s' cannot be null", propName)
		}
		return nil
	}
	
	// Type validation
	if err := v.validatePropertyType(propName, value, propDef); err != nil {
		return err
	}
	
	// Constraint validation
	if err := v.validatePropertyConstraints(propName, value, propDef.Constraints); err != nil {
		return err
	}
	
	return nil
}

// validatePropertyType validates the type of a property value
func (v *Validator) validatePropertyType(propName string, value interface{}, propDef models.PropertyDefinition) error {
	switch propDef.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("property '%s' must be a string", propName)
		}
	case "number":
		if !isNumeric(value) {
			return fmt.Errorf("property '%s' must be a number", propName)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("property '%s' must be a boolean", propName)
		}
	case "datetime":
		if !isDateTime(value) {
			return fmt.Errorf("property '%s' must be a valid datetime", propName)
		}
	case "object":
		if reflect.TypeOf(value).Kind() != reflect.Map {
			return fmt.Errorf("property '%s' must be an object", propName)
		}
		// Validate nested object if schema is provided
		if propDef.ObjectSchema != nil {
			if objMap, ok := value.(map[string]interface{}); ok {
				return v.validateProperties(objMap, propDef.ObjectSchema)
			}
		}
	case "array":
		if reflect.TypeOf(value).Kind() != reflect.Slice {
			return fmt.Errorf("property '%s' must be an array", propName)
		}
		// Validate array elements if element type is specified
		if propDef.ElementType != "" {
			return v.validateArrayElements(propName, value, propDef.ElementType)
		}
	case "vector":
		if err := v.validateVectorProperty(propName, value, propDef.VectorDim); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown property type '%s' for property '%s'", propDef.Type, propName)
	}
	
	return nil
}

// validateArrayElements validates array element types
func (v *Validator) validateArrayElements(propName string, value interface{}, elementType string) error {
	arrayValue := reflect.ValueOf(value)
	for i := 0; i < arrayValue.Len(); i++ {
		element := arrayValue.Index(i).Interface()
		elementPropDef := models.PropertyDefinition{Type: elementType}
		if err := v.validatePropertyType(fmt.Sprintf("%s[%d]", propName, i), element, elementPropDef); err != nil {
			return err
		}
	}
	return nil
}

// validateVectorProperty validates vector properties
func (v *Validator) validateVectorProperty(propName string, value interface{}, expectedDim int) error {
	// Check if it's a slice of numbers
	arrayValue := reflect.ValueOf(value)
	if arrayValue.Kind() != reflect.Slice {
		return fmt.Errorf("property '%s' must be a vector (array of numbers)", propName)
	}
	
	// Check dimensions
	if expectedDim > 0 && arrayValue.Len() != expectedDim {
		return fmt.Errorf("property '%s' must have %d dimensions, got %d", propName, expectedDim, arrayValue.Len())
	}
	
	// Check that all elements are numbers
	for i := 0; i < arrayValue.Len(); i++ {
		element := arrayValue.Index(i).Interface()
		if !isNumeric(element) {
			return fmt.Errorf("property '%s' element at index %d must be a number", propName, i)
		}
	}
	
	return nil
}

// validatePropertyConstraints validates property constraints
func (v *Validator) validatePropertyConstraints(propName string, value interface{}, constraints map[string]interface{}) error {
	if constraints == nil {
		return nil
	}
	
	for constraintType, constraintValue := range constraints {
		if err := v.validateConstraint(propName, value, constraintType, constraintValue); err != nil {
			return err
		}
	}
	
	return nil
}

// validateConstraint validates a specific constraint
func (v *Validator) validateConstraint(propName string, value interface{}, constraintType string, constraintValue interface{}) error {
	switch constraintType {
	case "min":
		if err := v.validateMinConstraint(propName, value, constraintValue); err != nil {
			return err
		}
	case "max":
		if err := v.validateMaxConstraint(propName, value, constraintValue); err != nil {
			return err
		}
	case "minLength":
		if err := v.validateMinLengthConstraint(propName, value, constraintValue); err != nil {
			return err
		}
	case "maxLength":
		if err := v.validateMaxLengthConstraint(propName, value, constraintValue); err != nil {
			return err
		}
	case "pattern":
		if err := v.validatePatternConstraint(propName, value, constraintValue); err != nil {
			return err
		}
	case "enum":
		if err := v.validateEnumConstraint(propName, value, constraintValue); err != nil {
			return err
		}
	}
	
	return nil
}

// validateMinConstraint validates minimum value constraint
func (v *Validator) validateMinConstraint(propName string, value interface{}, minValue interface{}) error {
	if !isNumeric(value) || !isNumeric(minValue) {
		return nil // Only apply to numeric values
	}
	
	val := toFloat64(value)
	min := toFloat64(minValue)
	
	if val < min {
		return fmt.Errorf("property '%s' must be >= %v", propName, min)
	}
	
	return nil
}

// validateMaxConstraint validates maximum value constraint
func (v *Validator) validateMaxConstraint(propName string, value interface{}, maxValue interface{}) error {
	if !isNumeric(value) || !isNumeric(maxValue) {
		return nil // Only apply to numeric values
	}
	
	val := toFloat64(value)
	max := toFloat64(maxValue)
	
	if val > max {
		return fmt.Errorf("property '%s' must be <= %v", propName, max)
	}
	
	return nil
}

// validateMinLengthConstraint validates minimum length constraint
func (v *Validator) validateMinLengthConstraint(propName string, value interface{}, minLength interface{}) error {
	length := getLength(value)
	if length == -1 {
		return nil // Only apply to strings and arrays
	}
	
	min, ok := minLength.(int)
	if !ok {
		if minFloat, ok := minLength.(float64); ok {
			min = int(minFloat)
		} else {
			return nil
		}
	}
	
	if length < min {
		return fmt.Errorf("property '%s' must have at least %d characters/elements", propName, min)
	}
	
	return nil
}

// validateMaxLengthConstraint validates maximum length constraint
func (v *Validator) validateMaxLengthConstraint(propName string, value interface{}, maxLength interface{}) error {
	length := getLength(value)
	if length == -1 {
		return nil // Only apply to strings and arrays
	}
	
	max, ok := maxLength.(int)
	if !ok {
		if maxFloat, ok := maxLength.(float64); ok {
			max = int(maxFloat)
		} else {
			return nil
		}
	}
	
	if length > max {
		return fmt.Errorf("property '%s' must have at most %d characters/elements", propName, max)
	}
	
	return nil
}

// validatePatternConstraint validates regex pattern constraint
func (v *Validator) validatePatternConstraint(propName string, value interface{}, pattern interface{}) error {
	strValue, ok := value.(string)
	if !ok {
		return nil // Only apply to strings
	}
	
	patternStr, ok := pattern.(string)
	if !ok {
		return nil
	}
	
	regex, err := regexp.Compile(patternStr)
	if err != nil {
		return fmt.Errorf("invalid pattern for property '%s': %v", propName, err)
	}
	
	if !regex.MatchString(strValue) {
		return fmt.Errorf("property '%s' does not match pattern '%s'", propName, patternStr)
	}
	
	return nil
}

// validateEnumConstraint validates enum constraint
func (v *Validator) validateEnumConstraint(propName string, value interface{}, enumValues interface{}) error {
	enumSlice, ok := enumValues.([]interface{})
	if !ok {
		return nil
	}
	
	for _, enumValue := range enumSlice {
		if reflect.DeepEqual(value, enumValue) {
			return nil
		}
	}
	
	return fmt.Errorf("property '%s' must be one of %v", propName, enumValues)
}

// validateEntityReferences validates that referenced entities exist
func (v *Validator) validateEntityReferences(ctx context.Context, relation *models.Relation) error {
	// Check from entity exists
	_, err := v.primaryStore.GetEntity(ctx, relation.FromEntityType, relation.FromEntityID)
	if err != nil {
		if utils.IsNotFound(err) {
			return utils.NewAppError(utils.CodeValidation, "from entity does not exist", err).
				WithDetail("entity_type", relation.FromEntityType).
				WithDetail("entity_id", relation.FromEntityID.String())
		}
		return err
	}
	
	// Check to entity exists
	_, err = v.primaryStore.GetEntity(ctx, relation.ToEntityType, relation.ToEntityID)
	if err != nil {
		if utils.IsNotFound(err) {
			return utils.NewAppError(utils.CodeValidation, "to entity does not exist", err).
				WithDetail("entity_type", relation.ToEntityType).
				WithDetail("entity_id", relation.ToEntityID.String())
		}
		return err
	}
	
	return nil
}

// validateCardinality validates relationship cardinality constraints
func (v *Validator) validateCardinality(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	switch schema.Cardinality {
	case models.OneToOne:
		return v.validateOneToOneCardinality(ctx, relation, schema)
	case models.OneToMany:
		return v.validateOneToManyCardinality(ctx, relation, schema)
	case models.ManyToOne:
		return v.validateManyToOneCardinality(ctx, relation, schema)
	case models.ManyToMany:
		// No additional constraints for many-to-many
		return nil
	default:
		return fmt.Errorf("unknown cardinality type: %s", schema.Cardinality)
	}
}

// validateOneToOneCardinality validates one-to-one cardinality
func (v *Validator) validateOneToOneCardinality(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	// Check if from entity already has a relation of this type
	fromRelations, err := v.primaryStore.GetRelationsByEntity(ctx, relation.FromEntityID, []string{relation.RelationType})
	if err != nil {
		return err
	}
	
	for _, rel := range fromRelations {
		if rel.ID != relation.ID && rel.FromEntityID == relation.FromEntityID {
			return utils.NewAppError(utils.CodeValidation, "one-to-one cardinality violation: from entity already has this relation type", nil).
				WithDetail("entity_id", relation.FromEntityID.String()).
				WithDetail("relation_type", relation.RelationType)
		}
	}
	
	// Check if to entity already has a relation of this type
	toRelations, err := v.primaryStore.GetRelationsByEntity(ctx, relation.ToEntityID, []string{relation.RelationType})
	if err != nil {
		return err
	}
	
	for _, rel := range toRelations {
		if rel.ID != relation.ID && rel.ToEntityID == relation.ToEntityID {
			return utils.NewAppError(utils.CodeValidation, "one-to-one cardinality violation: to entity already has this relation type", nil).
				WithDetail("entity_id", relation.ToEntityID.String()).
				WithDetail("relation_type", relation.RelationType)
		}
	}
	
	return nil
}

// validateOneToManyCardinality validates one-to-many cardinality
func (v *Validator) validateOneToManyCardinality(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	// Check if to entity already has a relation of this type (to entity can only have one)
	toRelations, err := v.primaryStore.GetRelationsByEntity(ctx, relation.ToEntityID, []string{relation.RelationType})
	if err != nil {
		return err
	}
	
	for _, rel := range toRelations {
		if rel.ID != relation.ID && rel.ToEntityID == relation.ToEntityID {
			return utils.NewAppError(utils.CodeValidation, "one-to-many cardinality violation: to entity already has this relation type", nil).
				WithDetail("entity_id", relation.ToEntityID.String()).
				WithDetail("relation_type", relation.RelationType)
		}
	}
	
	return nil
}

// validateManyToOneCardinality validates many-to-one cardinality
func (v *Validator) validateManyToOneCardinality(ctx context.Context, relation *models.Relation, schema *models.RelationshipSchema) error {
	// Check if from entity already has a relation of this type (from entity can only have one)
	fromRelations, err := v.primaryStore.GetRelationsByEntity(ctx, relation.FromEntityID, []string{relation.RelationType})
	if err != nil {
		return err
	}
	
	for _, rel := range fromRelations {
		if rel.ID != relation.ID && rel.FromEntityID == relation.FromEntityID {
			return utils.NewAppError(utils.CodeValidation, "many-to-one cardinality violation: from entity already has this relation type", nil).
				WithDetail("entity_id", relation.FromEntityID.String()).
				WithDetail("relation_type", relation.RelationType)
		}
	}
	
	return nil
}

// validatePropertyDefinition validates a property definition
func (v *Validator) validatePropertyDefinition(propName string, propDef models.PropertyDefinition) error {
	// Validate property type
	validTypes := []string{"string", "number", "boolean", "datetime", "object", "array", "vector"}
	isValidType := false
	for _, validType := range validTypes {
		if propDef.Type == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return fmt.Errorf("invalid property type '%s' for property '%s'", propDef.Type, propName)
	}
	
	// Validate vector dimension
	if propDef.Type == "vector" && propDef.VectorDim <= 0 {
		return fmt.Errorf("vector property '%s' must have a positive dimension", propName)
	}
	
	// Validate array element type
	if propDef.Type == "array" && propDef.ElementType != "" {
		elementPropDef := models.PropertyDefinition{Type: propDef.ElementType}
		if err := v.validatePropertyDefinition(propName+"[]", elementPropDef); err != nil {
			return err
		}
	}
	
	// Validate nested object schema
	if propDef.Type == "object" && propDef.ObjectSchema != nil {
		for nestedPropName, nestedPropDef := range propDef.ObjectSchema {
			if err := v.validatePropertyDefinition(propName+"."+nestedPropName, nestedPropDef); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// validateIndexConfig validates an index configuration
func (v *Validator) validateIndexConfig(index models.IndexConfig, properties models.PropertySchema) error {
	// Validate index type
	validIndexTypes := []string{"btree", "hash", "gin", "gist", "vector"}
	isValidType := false
	for _, validType := range validIndexTypes {
		if index.Type == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return fmt.Errorf("invalid index type '%s' for index '%s'", index.Type, index.Name)
	}
	
	// Validate that indexed fields exist in schema
	for _, field := range index.Fields {
		if _, exists := properties[field]; !exists {
			return fmt.Errorf("indexed field '%s' does not exist in schema", field)
		}
	}
	
	// Validate vector index configuration
	if index.Type == "vector" {
		if len(index.Fields) != 1 {
			return fmt.Errorf("vector index '%s' must index exactly one field", index.Name)
		}
		
		field := index.Fields[0]
		propDef := properties[field]
		if propDef.Type != "vector" {
			return fmt.Errorf("vector index '%s' can only be applied to vector properties", index.Name)
		}
		
		validVectorTypes := []string{"cosine", "l2", "ip"}
		if index.VectorType != "" {
			isValidVectorType := false
			for _, validType := range validVectorTypes {
				if index.VectorType == validType {
					isValidVectorType = true
					break
				}
			}
			if !isValidVectorType {
				return fmt.Errorf("invalid vector type '%s' for index '%s'", index.VectorType, index.Name)
			}
		}
	}
	
	return nil
}

// registerCustomValidators registers custom validation functions
func (v *Validator) registerCustomValidators() {
	v.validate.RegisterValidation("uuid", validateUUID)
	v.validate.RegisterValidation("urn", validateURN)
}

// Custom validation functions

func validateUUID(fl validator.FieldLevel) bool {
	_, err := uuid.Parse(fl.Field().String())
	return err == nil
}

func validateURN(fl validator.FieldLevel) bool {
	urn := fl.Field().String()
	// Basic URN validation - could be more sophisticated
	return len(urn) > 0 && len(urn) <= 500
}

// Helper functions

func isNumeric(value interface{}) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	default:
		return false
	}
}

func isDateTime(value interface{}) bool {
	switch v := value.(type) {
	case time.Time:
		return true
	case string:
		// Try to parse as RFC3339
		_, err := time.Parse(time.RFC3339, v)
		return err == nil
	case int64:
		// Unix timestamp
		return v > 0
	default:
		return false
	}
}

func toFloat64(value interface{}) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case uint:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return v
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0
}

func getLength(value interface{}) int {
	switch v := value.(type) {
	case string:
		return len(v)
	case []interface{}:
		return len(v)
	default:
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			return rv.Len()
		}
		return -1
	}
}