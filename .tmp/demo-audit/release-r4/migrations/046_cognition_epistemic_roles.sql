LOCK TABLE working_set_items, context_projection_selected_refs, context_projection_omitted_refs IN ACCESS EXCLUSIVE MODE;

ALTER TABLE working_set_items
    DROP CONSTRAINT working_set_items_role_check,
    ADD CONSTRAINT working_set_items_role_check CHECK (role IN (
        'user_authority', 'goal', 'objective', 'task', 'acceptance_criterion',
        'constraint', 'fact', 'hypothesis', 'decision', 'invariant', 'failure',
        'question', 'evidence', 'repository_evidence', 'dependency',
        'verification', 'historical'
    ));

ALTER TABLE context_projection_selected_refs
    DROP CONSTRAINT context_projection_selected_refs_role_check,
    ADD CONSTRAINT context_projection_selected_refs_role_check CHECK (role IN (
        'user_authority', 'goal', 'objective', 'task', 'acceptance_criterion',
        'constraint', 'fact', 'hypothesis', 'decision', 'invariant', 'failure',
        'question', 'evidence', 'repository_evidence', 'dependency',
        'verification', 'historical'
    ));

ALTER TABLE context_projection_omitted_refs
    DROP CONSTRAINT context_projection_omitted_refs_role_check,
    ADD CONSTRAINT context_projection_omitted_refs_role_check CHECK (role IN (
        'user_authority', 'goal', 'objective', 'task', 'acceptance_criterion',
        'constraint', 'fact', 'hypothesis', 'decision', 'invariant', 'failure',
        'question', 'evidence', 'repository_evidence', 'dependency',
        'verification', 'historical'
    ));
