import { Application } from "@hotwired/stimulus";
import RecyclrController from "./controllers/recyclr_controller";
import ChatController from "./controllers/chat_controller";
import ScrumController from "./controllers/scrum_controller";
import ProjectsController from "./controllers/projects_controller";
import ShellController from "./controllers/shell_controller";
import "../styles.css";
import { initI18n } from "./lib/i18n";
import { showToast } from "./lib/toast";

initI18n();

const application = Application.start();
application.register("recyclr", RecyclrController);
application.register("chat", ChatController);
application.register("scrum", ScrumController);
application.register("projects", ProjectsController);
application.register("shell", ShellController);

async function registerDeferredControllers(): Promise<void> {
  const [admin, adminDataSources, data, terminal, screen, projectChat, cardModal] = await Promise.all([
    import("./controllers/admin_controller"),
    import("./controllers/admin_data_sources_controller"),
    import("./controllers/data_controller"),
    import("./controllers/terminal_controller"),
    import("./controllers/screen_controller"),
    import("./controllers/project_chat_controller"),
    import("./controllers/card_modal_spa_controller"),
  ]);
  application.register("admin", admin.default);
  application.register("admin-data-sources", adminDataSources.default);
  application.register("data", data.default);
  application.register("terminal", terminal.default);
  application.register("screen", screen.default);
  application.register("project-chat", projectChat.default);
  application.register("card-modal-spa", cardModal.default);
}

void registerDeferredControllers().catch((error) => {
  console.error("Deferred UI controller loading failed", error);
  showToast("A workspace feature failed to load. Reload the page to retry.", "error");
});

export default application;
