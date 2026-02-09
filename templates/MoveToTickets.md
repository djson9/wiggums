<%*
const currentPath = tp.file.path(true);
const wsFolder = currentPath.split("/").slice(0, 2).join("/");
await tp.file.move(wsFolder + "/tickets/" + tp.file.title);
%>
