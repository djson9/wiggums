<%*
const title = await tp.system.prompt("New Note Name");
if (title) {
	const timestamp = Math.floor(Date.now() / 1000); // Unix timestamp in seconds
	const sanitizedTitle = title.replace(/\s+/g, '_');
	const fileName = `${timestamp}_${sanitizedTitle}`;
	
	const folder = "ticket_drafts";
	const template = tp.file.find_tfile("Ticket_Template");
    await tp.file.create_new(template, fileName, false, folder);
    const datetime = tp.date.now("YYYY-MM-DD HH:mm");
    tR += `${datetime} [[ticket_drafts/${fileName}|${title}]]`;
    // Open the new note after Buttons finishes appending the link
    setTimeout(async () => {
      const newFile = app.vault.getAbstractFileByPath(`${folder}/${fileName}.md`);
      if (newFile) await app.workspace.getLeaf('split').openFile(newFile);
    }, 200);
}
%>