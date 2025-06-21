-- Create entity_schemas table
CREATE TABLE entity_schemas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_type VARCHAR(100) NOT NULL UNIQUE,
    properties JSONB NOT NULL DEFAULT '{}',
    indexes JSONB DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    version INTEGER NOT NULL DEFAULT 1
);

-- Create indexes for entity_schemas
CREATE UNIQUE INDEX idx_entity_schemas_type ON entity_schemas(entity_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_entity_schemas_created_at ON entity_schemas(created_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_entity_schemas_deleted_at ON entity_schemas(deleted_at) WHERE deleted_at IS NOT NULL;

-- Create relationship_schemas table
CREATE TABLE relationship_schemas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    relationship_type VARCHAR(100) NOT NULL UNIQUE,
    from_entity_type VARCHAR(100) NOT NULL,
    to_entity_type VARCHAR(100) NOT NULL,
    properties JSONB DEFAULT '{}',
    cardinality VARCHAR(20) NOT NULL CHECK (cardinality IN ('one-to-one', 'one-to-many', 'many-to-one', 'many-to-many')),
    denormalization_config JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    version INTEGER NOT NULL DEFAULT 1
);

-- Create indexes for relationship_schemas
CREATE UNIQUE INDEX idx_relationship_schemas_type ON relationship_schemas(relationship_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_relationship_schemas_from_type ON relationship_schemas(from_entity_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_relationship_schemas_to_type ON relationship_schemas(to_entity_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_relationship_schemas_created_at ON relationship_schemas(created_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_relationship_schemas_deleted_at ON relationship_schemas(deleted_at) WHERE deleted_at IS NOT NULL;

-- Add triggers to automatically update updated_at
CREATE TRIGGER update_entity_schemas_updated_at BEFORE UPDATE ON entity_schemas
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_relationship_schemas_updated_at BEFORE UPDATE ON relationship_schemas
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create a function to validate vector dimensions in properties
CREATE OR REPLACE FUNCTION validate_vector_property(property_value JSONB, expected_dim INTEGER)
RETURNS BOOLEAN AS $$
DECLARE
    vector_array JSONB;
    actual_dim INTEGER;
BEGIN
    -- Check if the property is an array
    IF jsonb_typeof(property_value) != 'array' THEN
        RETURN FALSE;
    END IF;
    
    vector_array := property_value;
    actual_dim := jsonb_array_length(vector_array);
    
    -- Check if dimensions match
    IF actual_dim != expected_dim THEN
        RETURN FALSE;
    END IF;
    
    -- Check if all elements are numbers
    FOR i IN 0..actual_dim-1 LOOP
        IF jsonb_typeof(vector_array->i) NOT IN ('number') THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Create an index function for vector properties stored in JSONB
CREATE OR REPLACE FUNCTION jsonb_to_vector(vec JSONB) RETURNS vector AS $$
DECLARE
    result vector;
    dim INTEGER;
    i INTEGER;
    elements FLOAT[];
BEGIN
    dim := jsonb_array_length(vec);
    elements := ARRAY[]::FLOAT[];
    
    FOR i IN 0..dim-1 LOOP
        elements := array_append(elements, (vec->>i)::FLOAT);
    END LOOP;
    
    result := elements::vector;
    RETURN result;
END;
$$ LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE;