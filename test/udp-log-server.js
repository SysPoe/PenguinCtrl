import dgram from 'node:dgram';

const host = process.env.HOST || '0.0.0.0';
const oscPort = Number(process.env.OSC_PORT || 8001);
const udpPort = Number(process.env.UDP_PORT || 6553);

function align4(offset) {
  return (offset + 3) & ~3;
}

function readOscString(buffer, startOffset) {
  let end = startOffset;
  while (end < buffer.length && buffer[end] !== 0) end += 1;
  if (end >= buffer.length) {
    throw new Error('Unterminated OSC string');
  }
  const value = buffer.toString('utf8', startOffset, end);
  return { value, nextOffset: align4(end + 1) };
}

function readBlob(buffer, startOffset) {
  if (startOffset + 4 > buffer.length) {
    throw new Error('Truncated OSC blob length');
  }
  const size = buffer.readUInt32BE(startOffset);
  const blobStart = startOffset + 4;
  const blobEnd = blobStart + size;
  if (blobEnd > buffer.length) {
    throw new Error('Truncated OSC blob data');
  }
  return {
    value: buffer.subarray(blobStart, blobEnd),
    nextOffset: align4(blobEnd),
  };
}

function decodeOscMessage(buffer) {
  const addressResult = readOscString(buffer, 0);
  const typeTagResult = readOscString(buffer, addressResult.nextOffset);
  const typeTags = typeTagResult.value.startsWith(',') ? typeTagResult.value.slice(1) : typeTagResult.value;

  const args = [];
  let offset = typeTagResult.nextOffset;

  for (const tag of typeTags) {
    if (tag === 'i') {
      if (offset + 4 > buffer.length) throw new Error('Truncated OSC int');
      args.push(buffer.readInt32BE(offset));
      offset += 4;
    } else if (tag === 'f') {
      if (offset + 4 > buffer.length) throw new Error('Truncated OSC float');
      args.push(buffer.readFloatBE(offset));
      offset += 4;
    } else if (tag === 's') {
      const result = readOscString(buffer, offset);
      args.push(result.value);
      offset = result.nextOffset;
    } else if (tag === 'b') {
      const result = readBlob(buffer, offset);
      args.push(`blob(${result.value.length} bytes)`);
      offset = result.nextOffset;
    } else if (tag === 'T') {
      args.push(true);
    } else if (tag === 'F') {
      args.push(false);
    } else if (tag === 'N') {
      args.push(null);
    } else if (tag === 'I') {
      args.push(Infinity);
    } else {
      args.push(`[unsupported:${tag}]`);
    }
  }

  return { address: addressResult.value, args };
}

function formatPayload(msg) {
  try {
    const decoded = decodeOscMessage(msg);
    return `${decoded.address} ${JSON.stringify(decoded.args)}`;
  } catch {
    const text = msg.toString('utf8');
    const printable = /^[\x09\x0a\x0d\x20-\x7e]*$/.test(text);
    return printable ? text : msg.toString('hex');
  }
}

function createLoggerSocket(label, port) {
  const socket = dgram.createSocket('udp4');

  socket.on('listening', () => {
    const address = socket.address();
    console.log(`${label} logger listening on ${address.address}:${address.port}`);
  });

  socket.on('message', (msg, rinfo) => {
    const payload = formatPayload(msg);
    console.log(`[${label}] ${rinfo.address}:${rinfo.port} -> ${payload}`);
  });

  socket.on('error', (err) => {
    console.error(`${label} logger error:`, err);
  });

  socket.bind(port, host);
  return socket;
}

createLoggerSocket('OSC', oscPort);
createLoggerSocket('UDP', udpPort);

process.on('SIGINT', () => {
  console.log('Shutting down logger...');
  process.exit(0);
});