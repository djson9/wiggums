<%*
const request = await tp.system.prompt("Enter your additional request");
if (request) {
	const filePath = app.workspace.getActiveFile()?.path;
	const timestamp = tp.date.now("YYYY-MM-DD HH:mm");

	setTimeout(async () => {
		const file = app.vault.getAbstractFileByPath(filePath);
		if (!file) return;
		let content = await app.vault.read(file);

		const matches = content.match(/### Additional User Request #\d+/g);
		const num = matches ? matches.length + 1 : 1;

		content = content.replace("To be populated with further user request\n", "");

		const newSection = `\n### Additional User Request #${num} — ${timestamp}\n${request}\n`;

		const divider = "---\nBelow to be filled by agent";
		const idx = content.indexOf(divider);
		if (idx !== -1) {
			content = content.slice(0, idx) + newSection + content.slice(idx);
		}

		content = content.replace(/^Status:\s*.*/m, "Status: additional_user_request");

		await app.vault.modify(file, content);
	}, 300);
}
%>