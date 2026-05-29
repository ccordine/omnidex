import { Application } from "@hotwired/stimulus";
import GxController from "./controllers/gx_controller";
import ChatController from "./controllers/chat_controller";
import ScrumController from "./controllers/scrum_controller";
import ProjectsController from "./controllers/projects_controller";
import AdminController from "./controllers/admin_controller";
import "../styles.css";

const application = Application.start();
application.register("gx", GxController);
application.register("chat", ChatController);
application.register("scrum", ScrumController);
application.register("projects", ProjectsController);
application.register("admin", AdminController);

export default application;
