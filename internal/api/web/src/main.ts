import { Application } from "@hotwired/stimulus";
import GxController from "./controllers/gx_controller";
import ChatController from "./controllers/chat_controller";
import ScrumController from "./controllers/scrum_controller";
import ProjectsController from "./controllers/projects_controller";
import AdminController from "./controllers/admin_controller";
import ShellController from "./controllers/shell_controller";
import TerminalController from "./controllers/terminal_controller";
import ScreenController from "./controllers/screen_controller";
import "../styles.css";
import { initI18n } from "./lib/i18n";

initI18n();

const application = Application.start();
application.register("gx", GxController);
application.register("chat", ChatController);
application.register("scrum", ScrumController);
application.register("projects", ProjectsController);
application.register("admin", AdminController);
application.register("shell", ShellController);
application.register("terminal", TerminalController);
application.register("screen", ScreenController);

export default application;
