package roleplay

type SimulationAdminProjection struct {
	Schema       string                         `json:"schema"`
	WorldID      string                         `json:"world_id"`
	Scene        SceneSheet                     `json:"scene"`
	Participants []SceneParticipantProjection   `json:"participants"`
	Meters       []MeterDefinition              `json:"meters"`
	Commands     []InteractionCommandDefinition `json:"commands"`
	Items        []ItemTemplateDefinition       `json:"items"`
}

type NarrativeScene struct {
	Title               string                    `json:"title"`
	Description         string                    `json:"description"`
	ActiveCharacterName string                    `json:"active_character_name"`
	Initiative          SimulationInitiativeClock `json:"initiative"`
}

type NarrativePersona struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary"`
	Voice   string   `json:"voice"`
	Traits  []string `json:"traits"`
	Goals   []string `json:"goals"`
}

type NarrativeMeter struct {
	Name    string `json:"name"`
	Minimum int    `json:"minimum"`
	Maximum int    `json:"maximum"`
	Value   int    `json:"value"`
}

type NarrativeInventoryItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	UseDisplay  string `json:"use_display"`
}

type NarrativeSimulationProjection struct {
	Schema         string                   `json:"schema"`
	Scene          NarrativeScene           `json:"scene"`
	Participants   []string                 `json:"participants"`
	Viewpoint      NarrativePersona         `json:"viewpoint"`
	OngoingActions []NarrativeOngoingAction `json:"ongoing_actions,omitempty"`
	Meters         []NarrativeMeter         `json:"meters"`
	Inventory      []NarrativeInventoryItem `json:"inventory"`
	VisibleFacts   []string                 `json:"visible_facts"`
	Memories       []string                 `json:"memories"`
	RecentEvents   []string                 `json:"recent_events"`
}

type SimulationNarrativeAuthority struct {
	WorldID                   string   `json:"world_id"`
	SceneID                   string   `json:"scene_id"`
	SceneRevision             int64    `json:"scene_revision"`
	ViewpointID               string   `json:"viewpoint_id"`
	ParticipantIDs            []string `json:"participant_ids"`
	OngoingActionStateIDs     []string `json:"ongoing_action_state_ids,omitempty"`
	OngoingActionCharacterIDs []string `json:"ongoing_action_character_ids,omitempty"`
	MeterKeys                 []string `json:"meter_keys"`
	InventoryItemIDs          []string `json:"inventory_item_ids"`
	CanonEventIDs             []string `json:"canon_event_ids"`
	MemoryIDs                 []string `json:"memory_ids"`
	TransitionIDs             []string `json:"transition_ids"`
	Fingerprint               string   `json:"fingerprint"`
}
