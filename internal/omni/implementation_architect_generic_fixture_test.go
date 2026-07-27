package omni

import "fmt"

func deterministicGenericReactApp(contract ImplementationArchitectContract) string {
	title := "Omnidex React App"
	if promptRequestsNoteApp(architectContractPrompt(contract), contract.SourceToolTask) {
		title = "Notes App"
	}
	return fmt.Sprintf(`import React, { useState } from 'react';

const initialNotes = [
  { id: 1, title: 'Welcome note', body: 'Capture ideas in memory for this session.' },
];

export default function App() {
  const [notes, setNotes] = useState(initialNotes);
  const [title, setTitle] = useState('');
  const [body, setBody] = useState('');

  const addNote = () => {
    const trimmedTitle = title.trim();
    const trimmedBody = body.trim();
    if (!trimmedTitle && !trimmedBody) {
      return;
    }
    setNotes((current) => [...current, { id: Date.now(), title: trimmedTitle || 'Untitled', body: trimmedBody }]);
    setTitle('');
    setBody('');
  };

  const deleteNote = (id) => {
    setNotes((current) => current.filter((note) => note.id !== id));
  };

  return React.createElement('main', { className: 'app-shell notes-app' },
    React.createElement('header', { className: 'app-header' },
      React.createElement('h1', null, %q),
      React.createElement('p', null, 'In-memory notes with create and delete actions.')
    ),
    React.createElement('section', { className: 'note-form', 'aria-label': 'Note capture' },
      React.createElement('input', {
        type: 'text',
        placeholder: 'Note title',
        value: title,
        onChange: (event) => setTitle(event.target.value),
      }),
      React.createElement('textarea', {
        placeholder: 'Note body',
        value: body,
        onChange: (event) => setBody(event.target.value),
      }),
      React.createElement('button', { type: 'button', onClick: addNote }, 'Add note')
    ),
    React.createElement('section', { className: 'note-list', 'aria-label': 'Note list' },
      notes.map((note) => React.createElement('article', { key: note.id, className: 'note-card' },
        React.createElement('h2', null, note.title),
        React.createElement('p', null, note.body),
        React.createElement('button', { type: 'button', onClick: () => deleteNote(note.id) }, 'Delete note')
      ))
    )
  );
}
`, title)
}

func deterministicGenericReactAppCSS(contract ImplementationArchitectContract) string {
	return `:root {
  color: #0f172a;
  background: #f8fafc;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

body {
  margin: 0;
}

.app-shell {
  max-width: 960px;
  margin: 0 auto;
  padding: 24px;
}

.notes-app .app-header,
.note-form,
.note-list,
.note-card {
  display: grid;
  gap: 12px;
}

.note-form input,
.note-form textarea,
.note-card button,
.note-form button {
  font: inherit;
}

.note-form textarea {
  min-height: 120px;
}

.note-list {
  margin-top: 24px;
}

.note-card {
  padding: 16px;
  border: 1px solid #cbd5e1;
  border-radius: 12px;
  background: #ffffff;
}
`
}
