import path from 'node:path';

const MAX_UPLOAD_FILES = 10;
const MAX_UPLOAD_FILE_BYTES = 5 * 1024 * 1024;
const ALLOWED_IMAGE_EXTENSIONS_BY_MIME = new Map([
  ['image/jpeg', ['.jpg', '.jpeg']],
  ['image/png', ['.png']],
  ['image/webp', ['.webp']],
  ['image/gif', ['.gif']],
]);

export type UploadValidationResult =
  | { ok: true }
  | { ok: false; status: 400; error: string };

const IMAGE_SIGNATURES: Partial<Record<string, number[][]>> = {
  'image/png': [[0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]],
  'image/jpeg': [[0xff, 0xd8, 0xff]],
  'image/webp': [[0x52, 0x49, 0x46, 0x46]],
  'image/gif': [
    [0x47, 0x49, 0x46, 0x38, 0x37, 0x61],
    [0x47, 0x49, 0x46, 0x38, 0x39, 0x61],
  ],
};

function startsWithSignature(bytes: Uint8Array, signature: number[]) {
  return signature.every((value, index) => bytes[index] === value);
}

async function matchesImageSignature(file: File, mimeType: string) {
  const signatures = IMAGE_SIGNATURES[mimeType];
  if (!signatures) {
    return false;
  }

  const bytes = new Uint8Array(await file.slice(0, 12).arrayBuffer());
  if (mimeType === 'image/webp') {
    return (
      startsWithSignature(bytes, [0x52, 0x49, 0x46, 0x46]) &&
      startsWithSignature(bytes.slice(8), [0x57, 0x45, 0x42, 0x50])
    );
  }

  return signatures.some((signature) => startsWithSignature(bytes, signature));
}

export async function validateUploadFiles(files: File[]): Promise<UploadValidationResult> {
  if (files.length === 0) {
    return { ok: false, status: 400, error: '업로드할 파일이 없습니다.' };
  }

  if (files.length > MAX_UPLOAD_FILES) {
    return { ok: false, status: 400, error: '파일은 최대 10개까지 업로드할 수 있습니다.' };
  }

  for (const file of files) {
    if (file.size > MAX_UPLOAD_FILE_BYTES) {
      return { ok: false, status: 400, error: '파일 크기는 5MB 이하만 업로드할 수 있습니다.' };
    }

    const mimeType = file.type.toLowerCase();
    const allowedExtensions = ALLOWED_IMAGE_EXTENSIONS_BY_MIME.get(mimeType);
    if (!allowedExtensions) {
      return { ok: false, status: 400, error: '이미지 파일만 업로드할 수 있습니다.' };
    }

    const extension = path.extname(file.name).toLowerCase();
    if (!allowedExtensions.includes(extension)) {
      return { ok: false, status: 400, error: '허용되지 않는 파일 확장자입니다.' };
    }

    if (!(await matchesImageSignature(file, mimeType))) {
      return { ok: false, status: 400, error: '실제 이미지 파일만 업로드할 수 있습니다.' };
    }
  }

  return { ok: true };
}
