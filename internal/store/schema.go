package store

const currentSchemaVersion = 1

func schemaVersionValid(version int) bool {
	return version == currentSchemaVersion
}
