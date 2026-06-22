const MEDIA_EXTENSION = /\.(aac|aiff?|avi|flac|m4a|m4v|mkv|mov|mp3|mp4|ogg|opus|wav|webm)$/i;

export function createAudioUploads({ json, state, renderClipOptions, timecode, toast }) {
  async function uploadClip(file) {
    const payload = await json('/api/audio/upload', {
      method: 'POST',
      headers: {
        'Content-Type': file.type || 'application/octet-stream',
        'X-Filename': file.name,
      },
      body: file,
    });
    state.clips.push(payload);
    renderClipOptions(payload.path);
    await timecode.load();
  }

  const eventFiles = event => [...(event.dataTransfer?.files || [])];
  const mediaFiles = files => files.filter(file => file.type?.startsWith('audio/') || file.type?.startsWith('video/') || MEDIA_EXTENSION.test(file.name));

  function hasFiles(event) {
    return [...(event.dataTransfer?.types || [])].includes('Files');
  }

  function installDropTarget(target = document) {
    target.addEventListener('dragover', event => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = 'copy';
    });

    target.addEventListener('drop', event => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      const files = mediaFiles(eventFiles(event));
      if (!files.length) return;
      Promise.all(files.map(file => uploadClip(file)))
        .then(() => toast(`${files.length} media file${files.length === 1 ? '' : 's'} uploaded`))
        .catch(err => toast(err.message));
    });
  }

  return { installDropTarget, uploadClip };
}
