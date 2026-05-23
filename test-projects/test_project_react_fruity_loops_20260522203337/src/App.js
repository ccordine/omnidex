import React, { useMemo, useState } from 'react';

const channels = [
  { name: 'Kick', color: '#f97316' },
  { name: 'Snare', color: '#38bdf8' },
  { name: 'Hat', color: '#a3e635' },
  { name: 'Bass', color: '#c084fc' },
  { name: 'Lead', color: '#facc15' },
  { name: 'Vox Pad', color: '#fb7185' },
];

const notes = ['C5', 'B4', 'A4', 'G4', 'F4', 'E4', 'D4', 'C4'];
const pads = ['Kick', 'Clap', 'Hat', '808', 'Chord', 'Vox', 'Perc', 'FX'];
const h = React.createElement;

export default function App() {
  const [playing, setPlaying] = useState(false);
  const [tempo, setTempo] = useState(128);
  const [activeStep, setActiveStep] = useState(0);
  const [levels, setLevels] = useState(channels.map((_, index) => 72 - index * 6));
  const [pattern, setPattern] = useState(() =>
    channels.map((_, row) => Array.from({ length: 16 }, (_, step) => (step + row) % (row + 3) === 0))
  );

  const timeline = useMemo(() => Array.from({ length: 24 }, (_, index) => index), []);

  const toggleStep = (row, step) => {
    setPattern((current) =>
      current.map((steps, rowIndex) =>
        rowIndex === row ? steps.map((enabled, stepIndex) => (stepIndex === step ? !enabled : enabled)) : steps
      )
    );
    setActiveStep(step);
  };

  const updateLevel = (index, value) => {
    setLevels((current) => current.map((level, levelIndex) => (levelIndex === index ? Number(value) : level)));
  };

  return h('main', { className: 'studio-shell' },
    h('section', { className: 'transport-panel', 'aria-label': 'Transport controls' },
      h('div', null,
        h('p', { className: 'eyebrow' }, 'Omnidex Beat Studio'),
        h('h1', null, 'Fruity Loops Inspired Music Production Studio')
      ),
      h('div', { className: 'transport-controls' },
        h('button', { type: 'button', className: playing ? 'armed' : '', onClick: () => setPlaying((value) => !value) }, playing ? 'Pause' : 'Play'),
        h('button', { type: 'button', onClick: () => setActiveStep(0) }, 'Stop'),
        h('label', null,
          'Tempo control',
          h('input', { type: 'range', min: '70', max: '180', value: tempo, onChange: (event) => setTempo(event.target.value) }),
          h('strong', null, tempo + ' BPM')
        )
      )
    ),
    h('section', { className: 'timeline', 'aria-label': 'Visual timeline' },
      timeline.map((beat) => h('span', { key: beat, className: beat % 4 === 0 ? 'bar downbeat' : 'bar' }, beat + 1))
    ),
    h('section', { className: 'studio-grid' },
      h('section', { className: 'channel-rack', 'aria-label': 'Channel rack and pattern step sequencer' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Channel Rack'), h('span', null, 'Pattern step sequencer')),
        channels.map((channel, row) => h('div', { className: 'channel-row', key: channel.name },
          h('strong', { style: { '--channel': channel.color } }, channel.name),
          h('div', { className: 'steps' },
            pattern[row].map((enabled, step) => h('button', {
              type: 'button',
              key: step,
              className: enabled ? 'step active' : 'step',
              'aria-label': channel.name + ' step ' + (step + 1),
              onClick: () => toggleStep(row, step),
            }))
          )
        ))
      ),
      h('section', { className: 'mixer', 'aria-label': 'Mixer controls' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Mixer'), h('span', null, 'Levels and sends')),
        channels.map((channel, index) => h('label', { className: 'mixer-strip', key: channel.name },
          h('span', null, channel.name),
          h('input', { type: 'range', min: '0', max: '100', value: levels[index], onChange: (event) => updateLevel(index, event.target.value) }),
          h('strong', null, levels[index])
        ))
      )
    ),
    h('section', { className: 'lower-grid' },
      h('section', { className: 'piano-roll', 'aria-label': 'Piano roll note grid' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Piano Roll'), h('span', null, 'Note grid')),
        h('div', { className: 'note-grid' },
          notes.map((note) => h('div', { className: 'note-row', key: note },
            h('span', null, note),
            ...Array.from({ length: 16 }, (_, step) => h('button', { type: 'button', key: note + '-' + step, className: (step + note.charCodeAt(0)) % 5 === 0 ? 'note active' : 'note' }))
          ))
        )
      ),
      h('section', { className: 'pads', 'aria-label': 'Sample and instrument pads' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Sample/Instrument Pads'), h('span', null, 'Performance triggers')),
        h('div', { className: 'pad-grid' }, pads.map((pad) => h('button', { type: 'button', key: pad }, pad)))
      )
    )
  );
}
