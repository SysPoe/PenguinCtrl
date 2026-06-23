import dgram from 'node:dgram';

const udpSocket = dgram.createSocket('udp4');

function pad(value) {
  const raw = Buffer.from(`${String(value ?? '')}\0`, 'utf8');
  const padding = (4 - (raw.length % 4)) % 4;
  return padding ? Buffer.concat([raw, Buffer.alloc(padding)]) : raw;
}

function osc(address, args = []) {
  const tags = [','];
  const buffers = [];
  for (const arg of args) {
    if (typeof arg === 'number' && Number.isFinite(arg)) {
      const buf = Buffer.alloc(4);
      Number.isInteger(arg) ? (tags.push('i'), buf.writeInt32BE(arg, 0)) : (tags.push('f'), buf.writeFloatBE(arg, 0));
      buffers.push(buf);
    } else {
      tags.push('s');
      buffers.push(pad(arg));
    }
  }
  return Buffer.concat([pad(address), pad(tags.join('')), ...buffers]);
}

function port(value, fallback) {
  const n = Number(value);
  return Number.isFinite(n) ? Math.max(1, Math.min(65535, Math.round(n))) : fallback;
}

function parseCueNumber(raw) {
  const match = String(raw ?? '').trim().match(/^(\d+)(?:\.(\d+))?$/);
  if (!match) throw new Error(`Invalid cue number "${raw}"`);
  return `${Number(match[1])}.${((match[2] || '0') + '00').slice(0, 2)}`;
}

function sendUdp(payload, host, udpPort) {
  return new Promise((resolve, reject) => udpSocket.send(payload, udpPort, host, err => err ? reject(err) : resolve()));
}

export function resolveTemplate(value, cue) {
  const raw = String(value ?? '{cueNumber}');
  if (!raw.includes('{cueNumber')) return raw;
  const number = Number(cue?.number ?? cue?.cueNumber ?? cue?.num ?? 1);
  return raw.replace(/\{cueNumber(?:([+-])(\d+(?:\.\d+)?))?\}/g, (_m, op, amount) => String((number + (op === '-' ? -1 : 1) * Number(amount || 0)).toFixed(2).replace(/\.?0+$/, '')));
}

export function createOscDispatcher({ getTargets }) {
  function targets() {
    const raw = getTargets();
    return (Array.isArray(raw) && raw.length ? raw : [{ ip: '127.0.0.1', oscPort: 8000, remotePort: 6553 }]).map(t => ({
      ip: String(t?.ip || '127.0.0.1').trim() || '127.0.0.1',
      oscPort: port(t?.oscPort, 8000),
      remotePort: Number(t?.remotePort) === -1 ? -1 : port(t?.remotePort, 6553),
    }));
  }

  return async function dispatchCommand({ action, playback, cueNumber, level, setLevel, transport }) {
    if (action === 'none' && !setLevel) return;
    const jobs = targets().map(t => {
      const useRemote = transport !== 'osc' && t.remotePort > 0;
      if (useRemote) {
        const sends = [];
        if (action !== 'none') {
          const command = action === 'go' ? `${playback}G` : action === 'back' ? `${playback}S` : action === 'release' ? `${playback}R` : action === 'level' ? `${playback},${level}L` : `${playback},${parseCueNumber(cueNumber)}J`;
          sends.push(sendUdp(Buffer.from(command, 'ascii'), t.ip, t.remotePort));
        }
        if (setLevel && action !== 'level') sends.push(sendUdp(Buffer.from(`${playback},${level}L`, 'ascii'), t.ip, t.remotePort));
        return Promise.allSettled(sends);
      }
      const parsed = action === 'goto' ? parseCueNumber(cueNumber) : cueNumber;
      const address = action === 'go' ? `/pb/${playback}/go` : action === 'release' ? `/pb/${playback}/release` : action === 'level' ? `/pb/${playback}` : `/pb/${playback}/${parsed}`;
      const sends = action === 'none' ? [] : [sendUdp(osc(address, [action === 'level' ? level : 1]), t.ip, t.oscPort)];
      if (setLevel && action !== 'level') sends.push(sendUdp(osc(`/pb/${playback}`, [level]), t.ip, t.oscPort));
      return Promise.allSettled(sends);
    });
    await Promise.allSettled(jobs);
  };
}
