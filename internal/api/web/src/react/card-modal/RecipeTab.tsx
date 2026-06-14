import { useEffect, useState } from "react";
import { patchScrumCard } from "../../lib/scrum_api";
import { ActionButton, Panel, Select, TextArea } from "./common";
import type { CardModalChildProps } from "./types";

export function RecipeTab({ context, projectID, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const recipes = context.recipes ?? [];
  const [recipeID, setRecipeID] = useState(card.recipe_id || context.project_recipe_id || "");
  const [recipeJSON, setRecipeJSON] = useState(JSON.stringify(card.recipe && Object.keys(card.recipe).length > 0 ? card.recipe : context.project_recipe ?? {}, null, 2));

  useEffect(() => {
    setRecipeID(card.recipe_id || context.project_recipe_id || "");
    setRecipeJSON(JSON.stringify(card.recipe && Object.keys(card.recipe).length > 0 ? card.recipe : context.project_recipe ?? {}, null, 2));
  }, [card.id, card.updated_at, context.project_recipe_id]);

  return (
    <Panel title="Recipe">
      <div className="space-y-3">
        <div className="flex flex-wrap gap-2">
          <Select value={recipeID} onChange={(event) => setRecipeID(event.target.value)} className="min-w-[14rem]">
            <option value="">Project/default recipe</option>
            {recipes.map((recipe) => (
              <option key={recipe.id} value={recipe.id}>
                {recipe.id}
              </option>
            ))}
          </Select>
          <ActionButton
            onClick={() => {
              const recipe = recipes.find((entry) => entry.id === recipeID);
              if (recipe) setRecipeJSON(JSON.stringify(recipe, null, 2));
            }}
          >
            Load catalog
          </ActionButton>
          <ActionButton
            tone="primary"
            onClick={async () => {
              const updated = await runMutation("Saving recipe", async () => {
                let recipe: Record<string, unknown>;
                try {
                  recipe = JSON.parse(recipeJSON || "{}") as Record<string, unknown>;
                } catch {
                  throw new Error("Recipe JSON is invalid");
                }
                return patchScrumCard(card.id, { recipe_id: recipeID, recipe }, projectID);
              });
              if (updated) onCardUpdated(updated, { reloadContext: true });
            }}
          >
            Save recipe
          </ActionButton>
        </div>
        <TextArea value={recipeJSON} onChange={(event) => setRecipeJSON(event.target.value)} rows={20} className="w-full font-mono text-xs" />
      </div>
    </Panel>
  );
}
