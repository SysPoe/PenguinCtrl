import dgram from 'dgram';

// Configuration
const QUICKQ_IP = '192.168.200.1';
const QUICKQ_PORT = 8000;

// Helper to format raw strings into OSC-compliant binary buffers
function makeOscBuffer(address) {
    const stringLen = address.length;
    // OSC strings must be null-terminated and padded to a multiple of 4 bytes
    const padLen = 4 - (stringLen % 4);
    const bufLen = stringLen + padLen;
    
    const buffer = Buffer.alloc(bufLen);
    buffer.write(address, 'ascii');
    // Remaining bytes are automatically left as 0x00 (null padding)
    return buffer;
}

// Create UDP client
const client = dgram.createSocket('udp4');

// Target command (Fires "Go" on Playback 1)
const oscAddress = '/pb/1/go';
const message = makeOscBuffer(oscAddress);

console.log(`Sending OSC command: "${oscAddress}" to ${QUICKQ_IP}:${QUICKQ_PORT}...`);

client.send(message, 0, message.length, QUICKQ_PORT, QUICKQ_IP, (err) => {
    if (err) {
        console.error('Error sending packet:', err);
    } else {
        console.log('Packet sent successfully!');
    }
    client.close();
});
