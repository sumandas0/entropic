CREATE TABLE relations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    relation_type VARCHAR(100) NOT NULL,
    from_entity_id UUID NOT NULL,
    from_entity_type VARCHAR(100) NOT NULL,
    to_entity_id UUID NOT NULL,
    to_entity_type VARCHAR(100) NOT NULL,
    properties JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_relations_type ON relations(relation_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_relations_from_entity ON relations(from_entity_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_relations_to_entity ON relations(to_entity_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_relations_from_type ON relations(from_entity_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_relations_to_type ON relations(to_entity_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_relations_created_at ON relations(created_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_relations_deleted_at ON relations(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE INDEX idx_relations_from_entity_type ON relations(from_entity_id, relation_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_relations_to_entity_type ON relations(to_entity_id, relation_type) WHERE deleted_at IS NULL;

CREATE INDEX idx_relations_properties ON relations USING GIN (properties);

CREATE TRIGGER update_relations_updated_at BEFORE UPDATE ON relations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
