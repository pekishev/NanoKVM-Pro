export function isWebrtcSupported() {
  return 'RTCPeerConnection' in window;
}

export function getIceServers(): RTCIceServer[] {
  const host = window.location.hostname;
  if (
    host === 'localhost' ||
    host === '::1' ||
    /^127\./.test(host) ||
    /^10\./.test(host) ||
    /^192\.168\./.test(host) ||
    /^172\.(1[6-9]|2\d|3[01])\./.test(host)
  ) {
    return [];
  }

  return [{ urls: ['stun:stun.l.google.com:19302'] }];
}

export function isDecoderSupported() {
  return 'VideoDecoder' in window;
}

export function isH265Supported() {
  if ('getCapabilities' in RTCRtpReceiver) {
    const capabilities = RTCRtpReceiver.getCapabilities('video');
    if (capabilities && capabilities.codecs) {
      const h265Codec = capabilities.codecs.find((codec) => codec.mimeType === 'video/H265');
      return !!h265Codec;
    }
  }

  return false;
}

export function getSupportedVideoModes() {
  const webrtcSupported = isWebrtcSupported();
  const decoderSupported = isDecoderSupported();
  const h265Supported = isH265Supported();

  const videoModes = ['mjpeg'];

  if (webrtcSupported) {
    videoModes.push('h264-webrtc');

    if (h265Supported) {
      videoModes.push('h265-webrtc');
    }
  }

  if (decoderSupported) {
    videoModes.push('h264-direct');

    if (h265Supported) {
      videoModes.push('h265-direct');
    }
  }

  return videoModes;
}
