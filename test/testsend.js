import dgram from 'node:dgram';
import readline from 'node:readline';
import { Buffer } from 'node:buffer';

// Configuration Defaults
const DEFAULT_HOST = '127.0.0.1';
const DEFAULT_PORT = 8000;

// --- OSC Encoding Helpers ---

function align4(size) {
  return (size + 3) & ~3;
}

function writeString(value) {
  const len = Buffer.byteLength(value);
  const paddedLen = align4(len + 1); // +1 for null terminator
  const buffer = Buffer.alloc(paddedLen);
  buffer.write(value);
  return buffer;
}

function encodeOscMessage(address, args) {
  const addressBuf = writeString(address);
  
  let typeTags = ',';
  const valuesBufs = [];

  args.forEach(arg => {
    if (arg.type === 'i') {
      typeTags += 'i';
      const buf = Buffer.alloc(4);
      buf.writeInt32BE(Number(arg.value));
      valuesBufs.push(buf);
    } else if (arg.type === 'f') {
      typeTags += 'f';
      const buf = Buffer.alloc(4);
      buf.writeFloatBE(Number(arg.value));
      valuesBufs.push(buf);
    } else if (arg.type === 's') {
      typeTags += 's';
      valuesBufs.push(writeString(String(arg.value)));
    } else if (arg.type === 'b') {
      // Blobs: Int32 Size + Data + Padding
      typeTags += 'b';
      const data = Buffer.from(arg.value);
      const sizeBuf = Buffer.alloc(4);
      sizeBuf.writeUInt32BE(data.length);
      const paddedLen = align4(data.length);
      const paddedData = Buffer.alloc(paddedLen);
      data.copy(paddedData);
      valuesBufs.push(Buffer.concat([sizeBuf, paddedData]));
    } else if (arg.type === 'T') {
      typeTags += 'T';
    } else if (arg.type === 'F') {
      typeTags += 'F';
    } else if (arg.type === 'N') {
      typeTags += 'N';
    }
  });

  const typeTagBuf = writeString(typeTags);
  return Buffer.concat([addressBuf, typeTagBuf, ...valuesBufs]);
}

// --- Helper for decoding replies (Simplified from your logger) ---
function printReply(msg, rinfo) {
    // Try to print readable string, else hex
    const text = msg.toString('utf8');
    const isPrintable = /^[\x09\x0a\x0d\x20-\x7e]*$/.test(text);
    const content = isPrintable && text.startsWith('/') ? text : `[Hex: ${msg.toString('hex')}]`;
    console.log(`\n<< RECEIVED from ${rinfo.address}:${rinfo.port} | ${content}`);
    process.stdout.write('> '); // Restore prompt
}

// --- CLI Logic ---

const socket = dgram.createSocket('udp4');
const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
  prompt: '> '
});

let targetHost = DEFAULT_HOST;
let targetPort = DEFAULT_PORT;

console.log('--- Node.js Interactive OSC Sender ---');
console.log('Syntax: /address [type value] [type value] ...');
console.log('Types: i (int), f (float), s (string), b (blob), T (true), F (false), N (null)');
console.log('Commands: :config (change IP/Port), :exit');

// 1. Setup Socket Listener for Replies
socket.on('message', printReply);
socket.bind(() => {
    // Initial Configuration
    rl.question(`Target IP [${DEFAULT_HOST}]: `, (ip) => {
        targetHost = ip.trim() || DEFAULT_HOST;
        rl.question(`Target Port [${DEFAULT_PORT}]: `, (port) => {
            targetPort = Number(port.trim()) || DEFAULT_PORT;
            console.log(`\nReady! Sending to ${targetHost}:${targetPort}`);
            console.log('Example: /test i 123 s "hello world" T\n');
            rl.prompt();
        });
    });
});

// 2. Handle Input
rl.on('line', (line) => {
  const input = line.trim();
  if (!input) { rl.prompt(); return; }

  if (input === ':exit') {
    rl.close();
    return;
  }

  if (input === ':config') {
    rl.question(`New IP [${targetHost}]: `, (ip) => {
        if (ip.trim()) targetHost = ip.trim();
        rl.question(`New Port [${targetPort}]: `, (port) => {
            if (port.trim()) targetPort = Number(port.trim());
            console.log(`Updated: ${targetHost}:${targetPort}`);
            rl.prompt();
        });
    });
    return;
  }

  // Parse the command: /addr type value type value
  // Regex handles quoted strings: /path s "string with spaces" i 123
  const parts = input.match(/"[^"]+"|\S+/g); 
  
  if (!parts || !parts[0].startsWith('/')) {
    console.log('Error: Message must start with an OSC address (e.g., /test)');
    rl.prompt();
    return;
  }

  const address = parts[0];
  const args = [];
  
  // Parse arguments in pairs (type, value) or singletons (T, F, N)
  for (let i = 1; i < parts.length; i++) {
    const type = parts[i];
    
    // Handle valueless types
    if (['T', 'F', 'N'].includes(type)) {
        args.push({ type, value: null });
        continue;
    }

    // Handle valued types
    if (i + 1 >= parts.length) {
        console.log(`Error: Missing value for type '${type}'`);
        rl.prompt();
        return;
    }
    
    let value = parts[i+1];
    // Strip quotes if string
    if (value.startsWith('"') && value.endsWith('"')) {
        value = value.slice(1, -1);
    }

    args.push({ type, value });
    i++; // Skip value index
  }

  try {
    const buffer = encodeOscMessage(address, args);
    socket.send(buffer, targetPort, targetHost, (err) => {
        if (err) console.error('Send Error:', err);
        else console.log(`>> SENT to ${targetHost}:${targetPort} (${buffer.length} bytes)`);
        rl.prompt();
    });
  } catch (e) {
    console.error('Encoding Error:', e.message);
    rl.prompt();
  }

}).on('close', () => {
  console.log('Exiting...');
  socket.close();
  process.exit(0);
});
x``x