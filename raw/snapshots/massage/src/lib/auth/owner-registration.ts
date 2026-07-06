import { isNonEmptyString } from '@/lib/validation/input';

type OwnerRegistrationBody = {
  name?: string;
  email?: string;
  password?: string;
  businessName?: string;
  businessNumber?: string;
  phone?: string;
};

import { getPendingApprovalLoginHref } from '@/lib/auth/login-notice';

type RegisteredOwner = {
  id: string;
  email: string;
  role: string;
  status?: string;
};

type RegisterOwnerDeps = {
  registerOwner: (input: {
    name: string;
    email: string;
    password: string;
    businessName: string;
    businessNumber: string;
    phone: string;
  }) => Promise<RegisteredOwner>;
};

type OwnerRegistrationSuccessStateInput = {
  message?: string;
  nextUrl?: string;
  requiresApproval?: boolean;
};

function normalizeBusinessNumber(value: string) {
  const trimmed = value.trim();
  const digits = trimmed.replaceAll('-', '');

  if (/^\d{10}$/.test(digits)) {
    return `${digits.slice(0, 3)}-${digits.slice(3, 5)}-${digits.slice(5)}`;
  }

  return trimmed;
}

export function getOwnerRegistrationSuccessState(input: OwnerRegistrationSuccessStateInput) {
  return {
    message: input.message ?? '관리자 승인 후 로그인할 수 있습니다.',
    nextUrl: input.nextUrl ?? getPendingApprovalLoginHref(),
    requiresApproval: input.requiresApproval ?? true,
  };
}

export async function registerOwnerRoute(body: OwnerRegistrationBody, deps: RegisterOwnerDeps) {
  const requiredFields = [body.name, body.email, body.password, body.businessName, body.businessNumber, body.phone];
  if (!requiredFields.every(isNonEmptyString)) {
    return Response.json({ error: '필수 입력값이 누락되었습니다.' }, { status: 400 });
  }

  const user = await deps.registerOwner({
    name: body.name!.trim(),
    email: body.email!.trim(),
    password: body.password!,
    businessName: body.businessName!.trim(),
    businessNumber: normalizeBusinessNumber(body.businessNumber!),
    phone: body.phone!.trim(),
  });

  return Response.json(
    {
      user,
      ...getOwnerRegistrationSuccessState({}),
    },
    { status: 201 },
  );
}
