ALTER TABLE task_nodes
    ADD COLUMN inline_execution boolean NOT NULL DEFAULT false;

ALTER TABLE task_nodes
    ADD CONSTRAINT task_nodes_inline_execution_kind_check
    CHECK (NOT inline_execution OR kind = 'task');
