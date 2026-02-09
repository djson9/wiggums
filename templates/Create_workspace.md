<%*
const name = await tp.system.prompt("Workspace name");
if (name) {
	const directory = await tp.system.prompt("Target directory (absolute path)");
	if (directory) {
		const wsFolder = `workspaces/${name}`;

		// Create workspace directories
		await app.vault.createFolder(wsFolder).catch(() => {});
		await app.vault.createFolder(`${wsFolder}/tickets`).catch(() => {});
		await app.vault.createFolder(`${wsFolder}/ticket_drafts`).catch(() => {});

		// Create workspace index from template with Directory filled in
		const template = tp.file.find_tfile("workspace_index.sample");
		let content = await app.vault.read(template);
		content = content.replace("{{DIRECTORY}}", directory);
		await app.vault.create(`${wsFolder}/index.md`, content);

		// Create shortcuts file
		await app.vault.create(`${wsFolder}/shortcuts.md`, "# Shortcuts - Iteration Learnings\n\nRecord workflow shortcuts and iteration learnings here.\n");

		const datetime = tp.date.now("YYYY-MM-DD HH:mm");
		tR += `${datetime} [[workspaces/${name}/index|${name}]]`;

		// Open the workspace index
		setTimeout(async () => {
			const newFile = app.vault.getAbstractFileByPath(`${wsFolder}/index.md`);
			if (newFile) await app.workspace.getLeaf('split').openFile(newFile);
		}, 200);
	}
}
%>