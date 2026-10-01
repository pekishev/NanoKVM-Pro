import Queue from 'yocto-queue';

let canvas: OffscreenCanvas | null = null;
let ctx: OffscreenCanvasRenderingContext2D | null = null;
let rendering: boolean = false;
let decoder: VideoDecoder | null = null;
let config: { codec: string; optimizeForLatency: boolean } | null = null;

const frameQueue = new Queue<VideoFrame>();

self.onmessage = (event: MessageEvent) => {
  const { type, data, canvas: offscreenCanvas } = event.data;

  switch (type) {
    case 'init_h264':
    case 'init_h265':
      canvas = offscreenCanvas;
      ctx = canvas!.getContext('2d') as OffscreenCanvasRenderingContext2D;
      config = {
        codec: type === 'init_h265' ? 'hev1.1.6.L93.B0' : 'avc1.42E01F',
        optimizeForLatency: true
      };
      break;
    case 'ws_message':
      handleWsMessage(data);
      break;
    case 'error':
    case 'close':
      resetDecoder();
      break;
    case 'snapshot':
      postSnapshot();
      break;
  }
};

function postSnapshot() {
  if (!canvas || !ctx || canvas.width === 0 || canvas.height === 0) {
    self.postMessage({ type: 'snapshot', ok: false });
    return;
  }

  try {
    const image = ctx.getImageData(0, 0, canvas.width, canvas.height);
    self.postMessage(
      {
        type: 'snapshot',
        ok: true,
        width: canvas.width,
        height: canvas.height,
        buffer: image.data.buffer
      },
      [image.data.buffer as ArrayBuffer]
    );
  } catch {
    self.postMessage({ type: 'snapshot', ok: false });
  }
}

function handleWsMessage(message: ArrayBuffer) {
  try {
    if (message.byteLength < 9) {
      return;
    }

    const view = new DataView(message);
    const isKeyFrame = view.getUint8(0) === 1;
    const timestamp = Number(view.getBigUint64(1, true));
    const data = new Uint8Array(message, 9);

    if (!decoder && isKeyFrame) {
      initializeDecoder();
    }

    if (decoder?.state === 'configured') {
      decode(isKeyFrame, timestamp, data);
    }
  } catch (error) {
    console.error('Error processing WebSocket message in worker:', error);
  }
}

function initializeDecoder() {
  if (!self.VideoDecoder) {
    console.log('Error: WebCodecs API not supported in this worker.');
    return;
  }

  if (!config || (decoder && decoder.state !== 'unconfigured')) {
    return;
  }

  const init = {
    output: (frame: VideoFrame) => {
      frameQueue.enqueue(frame);
      if (frameQueue.size >= 10) {
        frameQueue.dequeue()?.close();
      }

      if (!rendering) {
        rendering = true;
        processFrameQueue();
      }
    },
    error: () => {
      resetDecoder();
    }
  };

  try {
    decoder = new VideoDecoder(init);
    decoder.configure(config);
  } catch (err) {
    console.log(err);
    decoder = null;
  }
}

function decode(isKeyFrame: boolean, timestamp: number, data: Uint8Array) {
  const chunk = new EncodedVideoChunk({
    type: isKeyFrame ? 'key' : 'delta',
    timestamp: timestamp,
    data: data
  });

  try {
    decoder?.decode(chunk);
  } catch (err: any) {
    if (err.name === 'TypeError' || err.message.includes('configured')) {
      resetDecoder();
    }
  }
}

function processFrameQueue() {
  const frame = frameQueue.dequeue();
  if (frame) {
    renderFrame(frame);
  }

  if (frameQueue.size > 0) {
    setTimeout(processFrameQueue, 0);
  } else {
    rendering = false;
  }
}

function renderFrame(frame: VideoFrame) {
  if (!canvas || !ctx) {
    frame.close();
    return;
  }

  if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
    canvas.width = frame.displayWidth;
    canvas.height = frame.displayHeight;
  }

  ctx.drawImage(frame, 0, 0, canvas.width, canvas.height);
  frame.close();
}

function resetDecoder() {
  if (decoder && decoder.state !== 'closed') {
    try {
      decoder.close();
    } catch (err) {
      console.log(err);
    }
  }

  decoder = null;
  rendering = false;

  Array.from(frameQueue.drain()).forEach((frame) => frame.close());
}
