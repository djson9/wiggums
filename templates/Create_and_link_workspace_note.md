<%*
const title = await tp.system.prompt("New Note Name");
if (title) {
	const timestamp = Math.floor(Date.now() / 1000);
	const sanitizedTitle = title.replace(/\s+/g, '_');
	const fileName = `${timestamp}_${sanitizedTitle}`;

	// Derive workspace folder from the current file's path
	const currentPath = tp.file.path(true);
	const wsFolder = currentPath.split("/").slice(0, 2).join("/");
	const folder = `${wsFolder}/ticket_drafts`;

	const template = tp.file.find_tfile("Ticket_Template");
	await tp.file.create_new(template, fileName, false, folder);
	const datetime = tp.date.now("YYYY-MM-DD HH:mm");
	tR += `${datetime} [[${folder}/${fileName}|${title}]]`;

	setTimeout(async () => {
		const newFile = app.vault.getAbstractFileByPath(`${folder}/${fileName}.md`);
		if (newFile) await app.workspace.getLeaf('split').openFile(newFile);
	}, 200);
}
%>