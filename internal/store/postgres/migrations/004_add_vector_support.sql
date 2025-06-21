-- Add support for vector searches on entity properties

-- Create a helper table to track vector indexes
CREATE TABLE vector_indexes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_type VARCHAR(100) NOT NULL,
    property_path TEXT NOT NULL,
    vector_dimension INTEGER NOT NULL,
    index_type VARCHAR(20) NOT NULL DEFAULT 'ivfflat',
    lists INTEGER DEFAULT 100, -- for ivfflat
    probes INTEGER DEFAULT 10, -- for ivfflat search
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_type, property_path)
);

-- Function to create vector index for a specific property
CREATE OR REPLACE FUNCTION create_vector_index(
    p_entity_type VARCHAR(100),
    p_property_path TEXT,
    p_dimension INTEGER,
    p_index_type VARCHAR(20) DEFAULT 'ivfflat'
) RETURNS VOID AS $$
DECLARE
    index_name TEXT;
    sql_query TEXT;
BEGIN
    -- Generate index name
    index_name := 'idx_vector_' || p_entity_type || '_' || replace(p_property_path, '.', '_');
    
    -- Build the CREATE INDEX query based on index type
    IF p_index_type = 'ivfflat' THEN
        sql_query := format(
            'CREATE INDEX %I ON entities USING ivfflat ((jsonb_to_vector(properties#>''{%s}'')::vector(%s)) vector_cosine_ops) WHERE entity_type = %L AND deleted_at IS NULL',
            index_name,
            p_property_path,
            p_dimension,
            p_entity_type
        );
    ELSIF p_index_type = 'hnsw' THEN
        sql_query := format(
            'CREATE INDEX %I ON entities USING hnsw ((jsonb_to_vector(properties#>''{%s}'')::vector(%s)) vector_cosine_ops) WHERE entity_type = %L AND deleted_at IS NULL',
            index_name,
            p_property_path,
            p_dimension,
            p_entity_type
        );
    ELSE
        RAISE EXCEPTION 'Unsupported index type: %', p_index_type;
    END IF;
    
    -- Execute the query
    EXECUTE sql_query;
    
    -- Record the index
    INSERT INTO vector_indexes (entity_type, property_path, vector_dimension, index_type)
    VALUES (p_entity_type, p_property_path, p_dimension, p_index_type);
END;
$$ LANGUAGE plpgsql;

-- Function to perform vector similarity search
CREATE OR REPLACE FUNCTION vector_search(
    p_entity_type VARCHAR(100),
    p_property_path TEXT,
    p_query_vector FLOAT[],
    p_limit INTEGER DEFAULT 10,
    p_filters JSONB DEFAULT NULL
) RETURNS TABLE (
    id UUID,
    urn VARCHAR(500),
    properties JSONB,
    similarity FLOAT
) AS $$
DECLARE
    query_vec vector;
    filter_clause TEXT DEFAULT '';
BEGIN
    -- Convert array to vector
    query_vec := p_query_vector::vector;
    
    -- Build filter clause if filters provided
    IF p_filters IS NOT NULL THEN
        filter_clause := format(' AND properties @> %L', p_filters);
    END IF;
    
    -- Execute the search query
    RETURN QUERY EXECUTE format(
        'SELECT e.id, e.urn, e.properties, 
                1 - (jsonb_to_vector(e.properties#>''{%s}'')::vector <=> %L::vector) as similarity
         FROM entities e
         WHERE e.entity_type = %L 
           AND e.deleted_at IS NULL
           AND e.properties#>''{%s}'' IS NOT NULL
           %s
         ORDER BY jsonb_to_vector(e.properties#>''{%s}'')::vector <=> %L::vector
         LIMIT %s',
        p_property_path,
        query_vec,
        p_entity_type,
        p_property_path,
        filter_clause,
        p_property_path,
        query_vec,
        p_limit
    );
END;
$$ LANGUAGE plpgsql;

-- Function to perform hybrid search (text + vector)
CREATE OR REPLACE FUNCTION hybrid_search(
    p_entity_type VARCHAR(100),
    p_text_query TEXT,
    p_vector_property TEXT,
    p_query_vector FLOAT[],
    p_text_weight FLOAT DEFAULT 0.5,
    p_vector_weight FLOAT DEFAULT 0.5,
    p_limit INTEGER DEFAULT 10
) RETURNS TABLE (
    id UUID,
    urn VARCHAR(500),
    properties JSONB,
    text_score FLOAT,
    vector_score FLOAT,
    combined_score FLOAT
) AS $$
DECLARE
    query_vec vector;
BEGIN
    query_vec := p_query_vector::vector;
    
    RETURN QUERY
    WITH text_results AS (
        SELECT e.id,
               ts_rank_cd(to_tsvector('english', e.properties::text), 
                         plainto_tsquery('english', p_text_query)) as text_rank
        FROM entities e
        WHERE e.entity_type = p_entity_type 
          AND e.deleted_at IS NULL
          AND to_tsvector('english', e.properties::text) @@ plainto_tsquery('english', p_text_query)
    ),
    vector_results AS (
        SELECT e.id,
               1 - (jsonb_to_vector(e.properties#>>p_vector_property)::vector <=> query_vec) as vector_sim
        FROM entities e
        WHERE e.entity_type = p_entity_type 
          AND e.deleted_at IS NULL
          AND e.properties#>>p_vector_property IS NOT NULL
    ),
    combined AS (
        SELECT COALESCE(t.id, v.id) as id,
               COALESCE(t.text_rank, 0) as text_score,
               COALESCE(v.vector_sim, 0) as vector_score
        FROM text_results t
        FULL OUTER JOIN vector_results v ON t.id = v.id
    )
    SELECT e.id, 
           e.urn, 
           e.properties,
           c.text_score,
           c.vector_score,
           (c.text_score * p_text_weight + c.vector_score * p_vector_weight) as combined_score
    FROM combined c
    JOIN entities e ON e.id = c.id
    ORDER BY combined_score DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;

-- Add GIN index for full-text search on properties
CREATE INDEX idx_entities_properties_text ON entities 
    USING GIN (to_tsvector('english', properties::text));