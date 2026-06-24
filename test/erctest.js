import { createSocket } from 'dgram';

// Configuration
const QUICKQ_IP = '10.99.231.233';
const REMOTE_PORT = 6553;

// Create UDP client
const client = createSocket('udp4');

// Command Syntax: [Playback Number],[Cue Number]J
const playback = 1; // Target Playback
const cue = 1;       // Target Cue ID
const command = `${playback},${cue}J`;

console.log(`Sending Jump Command: "${command}" to ${QUICKQ_IP}:${REMOTE_PORT}...`);

client.send(command, REMOTE_PORT, QUICKQ_IP, (err) => {
    if (err) {
        console.error('Error sending packet:', err);
    } else {
        console.log(`Successfully ordered Playback ${playback} to jump to Cue ${cue}!`);
    }
    client.close();
});
