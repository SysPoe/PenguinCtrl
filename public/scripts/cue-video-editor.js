import { renderClipOptions } from './cue-clip-options.js';

export function createVideoEditor({ $, state, json, toast }) {
  function renderOptions(selected = $('video-clip')?.value || '') {
    renderClipOptions($('video-clip'), state.videos || [], selected);
  }

  async function uploadClip(file) {
    const payload = await json('/api/video/upload', {
      method: 'POST',
      headers: {
        'Content-Type': file.type || 'application/octet-stream',
        'X-Filename': file.name,
      },
      body: file,
    });
    state.videos.push(payload);
    renderOptions(payload.path);
  }

  function fill(cue) {
    renderOptions(cue.videoClip || '');
    $('video-clip').value = cue.videoClip || '';
    $('video-style').value = cue.videoPlayStyle || 'replace';
    ['video-start:clipStart', 'video-end:clipEnd', 'video-fade-in:fadeIn', 'video-fade-out:fadeOut'].forEach(pair => {
      const [id, key] = pair.split(':');
      $(id).value = cue[key] ?? '';
    });
  }

  function collect(cue, num) {
    Object.assign(cue, {
      videoClip: $('video-clip').value || undefined,
      videoPlayStyle: $('video-style').value || 'replace',
      clipStart: num($('video-start')) ?? 0,
      clipEnd: num($('video-end')),
      fadeIn: num($('video-fade-in')) ?? 0,
      fadeOut: num($('video-fade-out')) ?? 0,
    });
  }

  function bind() {
    $('video-upload').onchange = event => {
      const file = event.target.files[0];
      if (file) uploadClip(file).catch(err => toast(err.message));
      event.target.value = '';
    };
  }

  return { bind, collect, fill, renderOptions };
}
